package com.lokiscale.loomspan.internal.core;

import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;
import com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory;

import java.time.Clock;
import java.util.List;
import java.util.function.Supplier;

public final class TestLoomspanSessions {

    private TestLoomspanSessions() {
    }

    public static LoomspanSession withId(String sessionId, String entrySkill, int maxDepth) {
        return new LoomspanSession(sessionId, entrySkill, maxDepth);
    }

    public static LoomspanSession withId(String sessionId,
                                        String entrySkill,
                                        int maxDepth,
                                        @Nullable Authentication authentication) {
        return new LoomspanSession(sessionId, entrySkill, maxDepth, authentication);
    }

    public static LoomspanSession withId(String sessionId,
                                        String entrySkill,
                                        int maxDepth,
                                        @Nullable Authentication authentication,
                                        TracePersistencePolicy persistencePolicy) {
        return new LoomspanSession(sessionId, entrySkill, maxDepth, authentication, persistencePolicy);
    }

    public static LoomspanSession withId(String sessionId,
                                        String entrySkill,
                                        int maxDepth,
                                        @Nullable Authentication authentication,
                                        TracePersistencePolicy persistencePolicy,
                                        Clock clock) {
        return new LoomspanSession(sessionId, entrySkill, maxDepth, authentication, persistencePolicy, clock);
    }

    public static LoomspanSession withTraceHandle(String sessionId,
                                                  String entrySkill,
                                                  Clock clock,
                                                  ExecutionTraceHandle traceHandle,
                                                  Supplier<String> failureIdSupplier) {
        return new LoomspanSession(
                sessionId, entrySkill, 8, List.of(), null, null, null, null, null,
                TracePersistencePolicy.ALWAYS, clock, NoOpExecutionObservationHandleFactory.INSTANCE,
                (ignoredSessionId, ignoredEntrySkill, ignoredPolicy, ignoredClock, ignoredObservation) -> traceHandle,
                failureIdSupplier);
    }

    public static LoomspanSession withObservation(String sessionId,
                                                   String entrySkill,
                                                   int maxDepth,
                                                   Clock clock,
                                                   ExecutionObservationHandleFactory observationHandleFactory) {
        return new LoomspanSession(
                sessionId, entrySkill, maxDepth, null, TracePersistencePolicy.ALWAYS, clock,
                observationHandleFactory);
    }
}
