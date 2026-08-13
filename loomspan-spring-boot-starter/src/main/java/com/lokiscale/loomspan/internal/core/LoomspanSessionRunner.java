package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention;
import com.lokiscale.loomspan.internal.runtime.trace.ConfiguredLimitsSnapshot;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.time.Clock;
import java.util.Objects;
import java.util.UUID;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.function.Consumer;
import java.util.function.Function;
import tools.jackson.databind.ObjectMapper;

public class LoomspanSessionRunner
{
    private final int maxDepth;
    private final TracePersistencePolicy tracePersistencePolicy;
    private final Clock clock;
    private final ExecutionObservationHandleFactory observationHandleFactory;
    private final InternalExecutionTraceHandleFactory traceHandleFactory;
    private final ObjectMapper canonicalTraceMapper;

    public LoomspanSessionRunner(int maxDepth)
    {
        this(maxDepth, TracePersistencePolicy.ONERROR, Clock.systemUTC());
    }

    public LoomspanSessionRunner(int maxDepth, TracePersistencePolicy tracePersistencePolicy)
    {
        this(maxDepth, tracePersistencePolicy, Clock.systemUTC());
    }

    public LoomspanSessionRunner(int maxDepth, TracePersistencePolicy tracePersistencePolicy, Clock clock)
    {
        this(maxDepth, tracePersistencePolicy, clock, NoOpExecutionObservationHandleFactory.INSTANCE);
    }

    public LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory)
    {
        this(maxDepth, tracePersistencePolicy, clock, observationHandleFactory,
                com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle::new);
    }

    public LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            CompletionGraceRetention completionGraceRetention)
    {
        this(maxDepth, tracePersistencePolicy, clock, observationHandleFactory,
                (sessionId, entrySkill, policy, handleClock, observationHandle) ->
                        new com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle(
                                sessionId,
                                entrySkill,
                                policy,
                                handleClock,
                                observationHandle,
                                Objects.requireNonNull(
                                        completionGraceRetention,
                                        "completionGraceRetention must not be null")));
    }

    public LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            CompletionGraceRetention completionGraceRetention,
            LoomspanProperties.Session.Quotas quotas)
    {
        this(maxDepth, tracePersistencePolicy, clock, observationHandleFactory,
                (sessionId, entrySkill, policy, handleClock, observationHandle) ->
                        new com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle(
                                sessionId,
                                entrySkill,
                                policy,
                                handleClock,
                                observationHandle,
                                Objects.requireNonNull(
                                        completionGraceRetention,
                                        "completionGraceRetention must not be null"),
                                ConfiguredLimitsSnapshot.from(quotas)));
    }

    public LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            CompletionGraceRetention completionGraceRetention,
            LoomspanProperties.Session.Quotas quotas,
            ObjectMapper canonicalTraceMapper)
    {
        this(maxDepth, tracePersistencePolicy, clock, observationHandleFactory,
                (sessionId, entrySkill, policy, handleClock, observationHandle) ->
                        new com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle(
                                sessionId, entrySkill, policy, handleClock, observationHandle,
                                Objects.requireNonNull(completionGraceRetention,
                                        "completionGraceRetention must not be null"),
                                ConfiguredLimitsSnapshot.from(quotas),
                                Objects.requireNonNull(canonicalTraceMapper,
                                        "canonicalTraceMapper must not be null")), canonicalTraceMapper);
    }

    LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory)
    {
        this(maxDepth, tracePersistencePolicy, clock, observationHandleFactory, traceHandleFactory,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().canonicalTrace());
    }

    private LoomspanSessionRunner(
            int maxDepth,
            TracePersistencePolicy tracePersistencePolicy,
            Clock clock,
            ExecutionObservationHandleFactory observationHandleFactory,
            InternalExecutionTraceHandleFactory traceHandleFactory,
            ObjectMapper canonicalTraceMapper)
    {
        if (maxDepth <= 0)
        {
            throw new IllegalArgumentException("maxDepth must be greater than zero");
        }

        this.maxDepth = maxDepth;
        this.tracePersistencePolicy = tracePersistencePolicy == null ? TracePersistencePolicy.ONERROR : tracePersistencePolicy;
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
        this.observationHandleFactory = Objects.requireNonNull(
                observationHandleFactory, "observationHandleFactory must not be null");
        this.traceHandleFactory = Objects.requireNonNull(traceHandleFactory, "traceHandleFactory must not be null");
        this.canonicalTraceMapper = Objects.requireNonNull(canonicalTraceMapper,
                "canonicalTraceMapper must not be null");
    }

    public void runWithNewSession(String entrySkill, Consumer<LoomspanSession> action)
    {
        runWithNewSession(entrySkill, null, action);
    }

    public void runWithNewSession(String entrySkill, @Nullable Authentication authentication, Consumer<LoomspanSession> action)
    {
        Objects.requireNonNull(action, "action must not be null");
        LoomspanSession session = new LoomspanSession(
                UUID.randomUUID().toString(),
                entrySkill,
                maxDepth,
                java.util.List.of(),
                null,
                null,
                null,
                null,
                authentication,
                tracePersistencePolicy,
                clock,
                observationHandleFactory,
                traceHandleFactory,
                () -> UUID.randomUUID().toString(),
                canonicalTraceMapper);

        LoomspanSessionHolder.runWithSession(session, () ->
        {
            Throwable failure = null;
            try
            {
                action.accept(session);
            }
            catch (RuntimeException | Error ex)
            {
                failure = ex;
                throw ex;
            }
            finally
            {
                completeSession(session, failure);
            }
        });
    }

    public <T> T callWithNewSession(String entrySkill, Function<LoomspanSession, T> action)
    {
        return callWithNewSession(entrySkill, null, action);
    }

    public <T> T callWithNewSession(String entrySkill, @Nullable Authentication authentication, Function<LoomspanSession, T> action)
    {
        Objects.requireNonNull(action, "action must not be null");
        LoomspanSession session = new LoomspanSession(
                UUID.randomUUID().toString(),
                entrySkill,
                maxDepth,
                java.util.List.of(),
                null,
                null,
                null,
                null,
                authentication,
                tracePersistencePolicy,
                clock,
                observationHandleFactory,
                traceHandleFactory,
                () -> UUID.randomUUID().toString(),
                canonicalTraceMapper);

        return LoomspanSessionHolder.callWithSession(session, () ->
        {
            Throwable failure = null;
            try
            {
                return action.apply(session);
            }
            catch (RuntimeException | Error ex)
            {
                failure = ex;
                throw ex;
            }
            finally
            {
                completeSession(session, failure);
            }
        });
    }

    private void finalizeSessionTrace(LoomspanSession session, @Nullable Throwable failure)
    {
        if (session.getExecutionTrace().completed())
        {
            return;
        }
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("entryPoint", "session-runner");
        metadata.put("remainingFrames", session.getFramesSnapshot().size());

        if (!session.getFramesSnapshot().isEmpty())
        {
            IllegalStateException openFrameFailure = new IllegalStateException(
                    "Cannot finalize standalone session '%s' with %d open execution frame(s)."
                            .formatted(session.getSessionId(), session.getFramesSnapshot().size()));
            String failureId = session.recordFailure(openFrameFailure,
                    Map.of("message", "Standalone session completed with open execution frames"));
            session.markTraceErrored();
            session.finalizeTrace(new TraceCompletion(
                    TraceOutcome.FAILED,
                    session.getSessionUsage().orElse(
                            com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty()),
                    failureId,
                    Map.copyOf(metadata)));
            throw openFrameFailure;
        }

        String terminalFailureId = null;
        if (failure != null)
        {
            LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
            TraceFailureMetadata.addTo(payload, failure, "Session execution failed");
            terminalFailureId = session.recordFailure(failure, Map.copyOf(payload));
        }

        session.finalizeTrace(new TraceCompletion(
                failure == null
                        ? TraceOutcome.SUCCEEDED
                        : (Thread.currentThread().isInterrupted() ? TraceOutcome.ABORTED : TraceOutcome.FAILED),
                session.getSessionUsage().orElse(
                        com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty()),
                terminalFailureId,
                Map.copyOf(metadata)));
    }

    private void completeSession(LoomspanSession session, @Nullable Throwable failure)
    {
        RuntimeException cleanupFailure = null;

        try
        {
            finalizeSessionTrace(session, failure);
        }
        catch (RuntimeException ex)
        {
            cleanupFailure = ex;
        }

        if (cleanupFailure != null)
        {
            if (failure != null)
            {
                if (!session.hasFailureRecordingFailure())
                {
                    failure.addSuppressed(cleanupFailure);
                }
            }
            else
            {
                throw cleanupFailure;
            }
        }
    }
}
