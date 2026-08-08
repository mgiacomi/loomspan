package com.lokiscale.loomspan.internal.observability;

import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention;
import com.lokiscale.loomspan.internal.runtime.trace.ImmediateCompletionRetention;
import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;

import java.io.IOException;
import java.nio.file.Path;
import java.time.Instant;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicReference;

public final class ObservabilityActivationCoordinator implements AutoCloseable
{
    public enum State { PENDING, ENABLED, DISABLED }

    private final AtomicReference<State> state = new AtomicReference<>(State.PENDING);
    private volatile ObservabilityRuntime runtime;
    private final ExecutionObservationHandleFactory observationFactory = this::createObservation;
    private final CompletionGraceRetention completionRetention = new GatedCompletionRetention();

    public State state() { return state.get(); }
    public boolean enabled() { return state.get() == State.ENABLED; }
    public Optional<ObservabilityRuntime> runtime() { return Optional.ofNullable(runtime); }
    public ExecutionObservationHandleFactory observationFactory() { return observationFactory; }
    public CompletionGraceRetention completionRetention() { return completionRetention; }

    public synchronized void enable(ObservabilityRuntime value)
    {
        if (state.get() != State.PENDING)
        {
            throw new IllegalStateException("Observability activation has already been decided");
        }
        runtime = java.util.Objects.requireNonNull(value, "runtime must not be null");
        state.set(State.ENABLED);
    }

    public synchronized void disable()
    {
        if (state.compareAndSet(State.PENDING, State.DISABLED))
        {
            return;
        }
        if (state.get() != State.DISABLED)
        {
            throw new IllegalStateException("Enabled observability cannot be disabled without closing");
        }
    }

    private ExecutionObservationHandle createObservation(String sessionId, String entrySkill)
    {
        ObservabilityRuntime current = runtime;
        return state.get() == State.ENABLED && current != null
                ? current.observationFactory().create(sessionId, entrySkill)
                : NoOpExecutionObservationHandleFactory.INSTANCE.create(sessionId, entrySkill);
    }

    @Override
    public synchronized void close()
    {
        ObservabilityRuntime current = runtime;
        runtime = null;
        state.set(State.DISABLED);
        if (current != null)
        {
            current.close();
        }
    }

    private final class GatedCompletionRetention implements CompletionGraceRetention
    {
        @Override
        public Optional<RetainedArtifact> retainOrDelete(
                Path artifactPath, Instant finalizedAt, String traceId, String sessionId) throws IOException
        {
            ObservabilityRuntime current = runtime;
            return state.get() == State.ENABLED && current != null
                    ? current.completionRetention().retainOrDelete(artifactPath, finalizedAt, traceId, sessionId)
                    : ImmediateCompletionRetention.INSTANCE.retainOrDelete(
                            artifactPath, finalizedAt, traceId, sessionId);
        }

        @Override
        public Optional<ArtifactLease> acquire(FinalizedTraceArtifact artifact) throws IOException
        {
            ObservabilityRuntime current = runtime;
            return state.get() == State.ENABLED && current != null
                    ? current.completionRetention().acquire(artifact)
                    : Optional.empty();
        }

        @Override
        public void close()
        {
            ObservabilityActivationCoordinator.this.close();
        }
    }
}
