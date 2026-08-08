package com.lokiscale.loomspan.internal.security;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import org.junit.jupiter.api.Test;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.Authentication;
import org.springframework.security.core.authority.AuthorityUtils;

import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class DefaultAccessGuardTest {

    private final DefaultAccessGuard accessGuard = new DefaultAccessGuard();

    @Test
    void allowsUnprotectedCapabilityWithoutAuthentication() {
        CapabilityMetadata capability = capability("public.skill", Set.of());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);

        assertThat(accessGuard.canAccess(capability, session, null)).isTrue();
        accessGuard.checkAccess(capability, session, null);
    }

    @Test
    void deniesProtectedCapabilityWithoutInvocationOrSessionAuthentication() {
        CapabilityMetadata capability = capability("protected.skill", Set.of("ROLE_ALLOWED"));
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);

        assertThat(accessGuard.canAccess(capability, session, null)).isFalse();
        assertThatThrownBy(() -> accessGuard.checkAccess(capability, session, null))
                .isInstanceOf(AccessDeniedException.class)
                .hasMessageContaining("protected.skill");
    }

    @Test
    void usesSessionAuthenticationWhenInvocationAuthenticationIsNull() {
        CapabilityMetadata capability = capability("protected.skill", Set.of("ROLE_ALLOWED"));
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
        session.setAuthentication(authentication("ROLE_ALLOWED"));

        assertThat(accessGuard.resolveAuthentication(null, session)).isEqualTo(session.getAuthentication().orElseThrow());
        assertThat(accessGuard.canAccess(capability, session, null)).isTrue();
    }

    @Test
    void prefersInvocationAuthenticationOverSessionAuthentication() {
        CapabilityMetadata capability = capability("protected.skill", Set.of("ROLE_ALLOWED"));
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
        session.setAuthentication(authentication("ROLE_ALLOWED"));
        Authentication invocationAuthentication = authentication("ROLE_OTHER");

        assertThat(accessGuard.resolveAuthentication(invocationAuthentication, session)).isEqualTo(invocationAuthentication);
        assertThat(accessGuard.canAccess(capability, session, invocationAuthentication)).isFalse();
        assertThat(accessGuard.canAccess(capability, session, authentication("ROLE_ALLOWED"))).isTrue();
    }

    private static CapabilityMetadata capability(String name, Set<String> roles) {
        return new CapabilityMetadata(
                "yaml:" + name,
                name,
                name,
                new SkillExecutionDescriptor("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium"),
                roles,
                arguments -> "ok",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic(name, name),
                null);
    }

    private static Authentication authentication(String... authorities) {
        return UsernamePasswordAuthenticationToken.authenticated(
                "user",
                "pw",
                AuthorityUtils.createAuthorityList(authorities));
    }
}
