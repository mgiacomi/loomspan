package com.lokiscale.loomspan.internal.core;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LoomspanSessionHolderTest {

    @Test
    void throwsWhenCurrentSessionIsAccessedOutsideScope() {
        assertThatThrownBy(LoomspanSession::getCurrentSession)
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("No active Loomspan session is bound to the current execution.");
    }

    @Test
    void returnsCurrentSessionInsideScopedBoundary() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 4);

        LoomspanSession resolved = LoomspanSessionHolder.callWithSession(session, LoomspanSession::getCurrentSession);

        assertThat(resolved).isSameAs(session);
    }
}
