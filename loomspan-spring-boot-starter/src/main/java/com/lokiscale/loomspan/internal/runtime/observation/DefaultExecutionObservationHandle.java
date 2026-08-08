package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalog;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalogEntry;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.atomic.AtomicBoolean;

final class DefaultExecutionObservationHandle implements ExecutionObservationHandle
{
    private static final Logger LOGGER = LoggerFactory.getLogger(DefaultExecutionObservationHandle.class);

    private final String sessionId;
    private final LiveActivityProjector projector;
    private final ActiveExecutionRegistry registry;
    private final ActivityReplayBuffer replayBuffer;
    private final LiveMonitoringAvailability availability;
    private final FinalizedTraceCatalog traceCatalog;
    private final LiveActivitySignal signal;
    private final ExecutionProjectionState state;
    private final AtomicBoolean closed = new AtomicBoolean();

    private ExecutionActivity heldCompletion;
    private String traceId;

    DefaultExecutionObservationHandle(
            String sessionId,
            String entrySkill,
            LiveActivityProjector projector,
            ActiveExecutionRegistry registry,
            ActivityReplayBuffer replayBuffer,
            LiveMonitoringAvailability availability,
            FinalizedTraceCatalog traceCatalog,
            LiveActivitySignal signal)
    {
        this.sessionId = requireNonBlank(sessionId, "sessionId");
        this.projector = Objects.requireNonNull(projector, "projector must not be null");
        this.registry = Objects.requireNonNull(registry, "registry must not be null");
        this.replayBuffer = Objects.requireNonNull(replayBuffer, "replayBuffer must not be null");
        this.availability = Objects.requireNonNull(availability, "availability must not be null");
        this.traceCatalog = Objects.requireNonNull(traceCatalog, "traceCatalog must not be null");
        this.signal = Objects.requireNonNull(signal, "signal must not be null");
        this.state = new ExecutionProjectionState(sessionId, requireNonBlank(entrySkill, "entrySkill"));
    }

    @Override
    public synchronized void recordAppended(TraceRecord record)
    {
        if (closed.get() || !availability.isAvailable())
        {
            return;
        }
        LiveActivityProjector.Projection projection;
        try
        {
            traceId = record.traceId();
            projection = projector.project(state, record);
        }
        catch (RuntimeException ex)
        {
            failClosed("PROJECTION_FAILED", ex);
            return;
        }
        try
        {
            registry.replace(projection.snapshot());
        }
        catch (RuntimeException ex)
        {
            failClosed("REGISTRY_UPDATE_FAILED", ex);
            return;
        }
        if (projection.heldTerminal() != null)
        {
            heldCompletion = projection.heldTerminal();
            return;
        }
        if (projection.activity() != null)
        {
            try
            {
                replayBuffer.append(projection.activity());
                signalActivity();
            }
            catch (RuntimeException ex)
            {
                failClosed("REPLAY_PUBLICATION_FAILED", ex);
            }
        }
    }

    @Override
    public void close(ObservationCompletionDisposition disposition)
    {
        Objects.requireNonNull(disposition, "disposition must not be null");
        if (!closed.compareAndSet(false, true))
        {
            return;
        }
        try
        {
            if (!availability.isAvailable())
            {
                return;
            }
            if (disposition.status()
                    == ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED)
            {
                publishSuccessfulTerminal(disposition);
            }
            else
            {
                heldCompletion = null;
                replayBuffer.append(exceptionalTerminal(disposition));
                signalActivity();
            }
        }
        catch (RuntimeException ex)
        {
            failClosed("TERMINAL_PUBLICATION_FAILED", ex);
        }
        finally
        {
            try
            {
                registry.remove(sessionId);
            }
            catch (RuntimeException ex)
            {
                failClosed("REGISTRY_UPDATE_FAILED", ex);
            }
            heldCompletion = null;
        }
    }

    private void publishSuccessfulTerminal(ObservationCompletionDisposition disposition)
    {
        ExecutionActivity completion = heldCompletion;
        heldCompletion = null;
        if (completion == null)
        {
            throw new IllegalStateException("Canonical completion was not observed before successful close");
        }
        ExecutionActivity enriched;
        if (disposition.finalizedArtifact().isEmpty())
        {
            enriched = completion.withTraceAvailability("UNAVAILABLE", "NOT_RETAINED", null);
        }
        else
        {
            try
            {
                FinalizedTraceCatalogEntry entry = traceCatalog.publish(disposition.finalizedArtifact().orElseThrow());
                enriched = completion.withTraceAvailability(
                        "AVAILABLE", null, entry.applicationTraceExpiresAt());
            }
            catch (RuntimeException ex)
            {
                LOGGER.warn(
                        "Trace catalog publication failed sessionId={} traceId={} exceptionClass={}",
                        sessionId,
                        traceId == null ? "unknown" : traceId,
                        ex.getClass().getName());
                enriched = completion.withTraceAvailability(
                        "UNAVAILABLE", "CATALOG_PUBLICATION_FAILED", null);
            }
        }
        replayBuffer.append(enriched);
        signalActivity();
    }

    private ExecutionActivity exceptionalTerminal(ObservationCompletionDisposition disposition)
    {
        Map<String, Object> details = new LinkedHashMap<>();
        details.put("reason", ObservationCompletionDisposition.Status.CORE_FINALIZATION_FAILED.name());
        details.put("applicationTraceAvailability", "UNAVAILABLE");
        details.put("applicationTraceUnavailableReason", "CORE_FINALIZATION_FAILED");
        String executionStatus = disposition.outcome() == null ? null : disposition.outcome().name();
        if (disposition.outcome() != null)
        {
            details.put("outcome", executionStatus);
        }
        String activityTraceId = traceId == null ? "unknown" : traceId;
        String summary = "Execution observation ended during core finalization";
        int weight = 128
                + ExecutionObservationLimits.utf8Weight(sessionId)
                + ExecutionObservationLimits.utf8Weight(activityTraceId)
                + ExecutionObservationLimits.utf8Weight(executionStatus)
                + ExecutionObservationLimits.utf8Weight(ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED.name())
                + ExecutionObservationLimits.utf8Weight(summary)
                + details.entrySet().stream()
                        .mapToInt(entry -> ExecutionObservationLimits.utf8Weight(entry.getKey())
                                + ExecutionObservationLimits.utf8Weight(String.valueOf(entry.getValue())) + 8)
                        .sum();
        return new ExecutionActivity(
                0L,
                sessionId,
                activityTraceId,
                null,
                disposition.closedAt(),
                ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED,
                null,
                null,
                null,
                null,
                executionStatus,
                summary,
                Map.copyOf(details),
                Math.max(1, weight));
    }

    private void failClosed(String operation, RuntimeException failure)
    {
        if (availability.fail(operation, failure))
        {
            LOGGER.error(
                    "Live monitoring unavailable operation={} sessionId={} traceId={} exceptionClass={}",
                    operation,
                    sessionId,
                    traceId == null ? "unknown" : traceId,
                    failure.getClass().getName());
            try
            {
                signal.liveUnavailable();
            }
            catch (RuntimeException signalFailure)
            {
                LOGGER.warn(
                        "Live monitoring signal failed operation=LIVE_UNAVAILABLE sessionId={} traceId={} exceptionClass={}",
                        sessionId,
                        traceId == null ? "unknown" : traceId,
                        signalFailure.getClass().getName());
            }
        }
    }

    private void signalActivity()
    {
        try
        {
            signal.activityAvailable();
        }
        catch (RuntimeException failure)
        {
            LOGGER.warn(
                    "Live activity signal failed sessionId={} traceId={} exceptionClass={}",
                    sessionId,
                    traceId == null ? "unknown" : traceId,
                    failure.getClass().getName());
        }
    }

    private static String requireNonBlank(String value, String name)
    {
        Objects.requireNonNull(value, name + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(name + " must not be blank");
        }
        return value;
    }
}
