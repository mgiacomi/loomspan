package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalog;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalogEntry;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.TraceCatalogSlice;

import java.util.Optional;
import java.util.Objects;

public final class DefaultExecutionObservationHandleFactory implements ExecutionObservationHandleFactory
{
    private final LiveActivityProjector projector;
    private final ActiveExecutionRegistry registry;
    private final ActivityReplayBuffer replayBuffer;
    private final LiveMonitoringAvailability availability;
    private final FinalizedTraceCatalog traceCatalog;
    private final LiveActivitySignal signal;

    public DefaultExecutionObservationHandleFactory()
    {
        this(
                new LiveActivityProjector(),
                new InMemoryActiveExecutionRegistry(),
                new InMemoryActivityReplayBuffer(),
                new LiveMonitoringAvailability(),
                unavailableCatalog(),
                LiveActivitySignal.NO_OP);
    }

    public DefaultExecutionObservationHandleFactory(
            LiveActivityProjector projector,
            ActiveExecutionRegistry registry,
            ActivityReplayBuffer replayBuffer,
            LiveMonitoringAvailability availability,
            FinalizedTraceCatalog traceCatalog,
            LiveActivitySignal signal)
    {
        this.projector = Objects.requireNonNull(projector, "projector must not be null");
        this.registry = Objects.requireNonNull(registry, "registry must not be null");
        this.replayBuffer = Objects.requireNonNull(replayBuffer, "replayBuffer must not be null");
        this.availability = Objects.requireNonNull(availability, "availability must not be null");
        this.traceCatalog = Objects.requireNonNull(traceCatalog, "traceCatalog must not be null");
        this.signal = Objects.requireNonNull(signal, "signal must not be null");
    }

    @Override
    public ExecutionObservationHandle create(String sessionId, String entrySkill)
    {
        return new DefaultExecutionObservationHandle(
                sessionId, entrySkill, projector, registry, replayBuffer, availability, traceCatalog, signal);
    }

    public ActiveExecutionRegistry registry()
    {
        return registry;
    }

    public ActivityReplayBuffer replayBuffer()
    {
        return replayBuffer;
    }

    public LiveMonitoringAvailability availability()
    {
        return availability;
    }

    public FinalizedTraceCatalog traceCatalog()
    {
        return traceCatalog;
    }

    static FinalizedTraceCatalog unavailableCatalog()
    {
        return new FinalizedTraceCatalog()
        {
            @Override
            public FinalizedTraceCatalogEntry publish(FinalizedTraceArtifact artifact)
            {
                throw new IllegalStateException("No finalized trace catalog is configured");
            }

            @Override
            public Optional<FinalizedTraceCatalogEntry> find(String traceId)
            {
                return Optional.empty();
            }

            @Override
            public Optional<FinalizedTraceCatalog.ArtifactAcquisition> acquire(String traceId)
            {
                return Optional.empty();
            }

            @Override
            public TraceCatalogSlice list(long highWaterOrdinal, long beforeOrdinal, int limit)
            {
                return new TraceCatalogSlice(0, java.util.List.of());
            }

            @Override
            public int catalogedTraceCount()
            {
                return 0;
            }

            @Override
            public void close()
            {
            }
        };
    }
}
