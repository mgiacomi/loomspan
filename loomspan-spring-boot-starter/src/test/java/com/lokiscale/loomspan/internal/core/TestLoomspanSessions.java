package com.lokiscale.loomspan.internal.core;

import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.time.Clock;

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
}
