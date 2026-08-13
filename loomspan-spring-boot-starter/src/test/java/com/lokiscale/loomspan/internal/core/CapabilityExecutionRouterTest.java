package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.PlanSnapshot;
import com.lokiscale.loomspan.internal.security.DefaultAccessGuard;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.AuthorityUtils;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class CapabilityExecutionRouterTest {

    @Test
    void restoresParentPlanViaStateService() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        ExecutionCoordinator coordinator = mock(ExecutionCoordinator.class);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory(Map.of("executionCoordinator", coordinator));
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                beanFactory.getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:child",
                "child.llm.skill",
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child"),
                null);
        PlanSnapshot snapshot = PlanSnapshot.of(null);
        com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot evidenceSnapshot = com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot.of(java.util.Set.of("parsed_invoice"));

        when(stateService.snapshotPlan(session)).thenReturn(snapshot);
        when(stateService.snapshotSuccessfulSkills(session)).thenReturn(evidenceSnapshot);
        when(coordinator.execute(eq("child.llm.skill"), org.mockito.ArgumentMatchers.anyString(), org.mockito.ArgumentMatchers.anyMap(), eq(session), eq(null)))
                .thenReturn("child result");

        Object result = router.execute(capability, Map.of("topic", "mars"), session, null);

        assertThat(result).isEqualTo("child result");
        verify(stateService).restorePlan(session, snapshot);
        verify(stateService).restoreSuccessfulSkills(session, evidenceSnapshot);
    }

    @Test
    void deniesProtectedCapabilityWithoutMatchingAuthority() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                new StaticListableBeanFactory().getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:child",
                "child.llm.skill",
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of("ROLE_ALLOWED"),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child"),
                "targetBean#deterministicTarget");

        assertThatThrownBy(() -> router.execute(
                capability,
                Map.of("topic", "mars"),
                session,
                UsernamePasswordAuthenticationToken.authenticated(
                        "user",
                        "pw",
                        AuthorityUtils.createAuthorityList("ROLE_OTHER"))))
                .isInstanceOf(AccessDeniedException.class)
                .hasMessageContaining("child.llm.skill");
    }

    @Test
    void authorizesNestedYamlDelegationUsingSessionFallbackAndRestoresPlan() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        ExecutionCoordinator coordinator = mock(ExecutionCoordinator.class);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory(Map.of("executionCoordinator", coordinator));
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                beanFactory.getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        session.setAuthentication(UsernamePasswordAuthenticationToken.authenticated(
                "user",
                "pw",
                AuthorityUtils.createAuthorityList("ROLE_ALLOWED")));
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:child",
                "child.llm.skill",
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of("ROLE_ALLOWED"),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child"),
                null);
        PlanSnapshot snapshot = PlanSnapshot.of(null);
        com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot evidenceSnapshot = com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot.of(java.util.Set.of("parsed_invoice"));

        when(stateService.snapshotPlan(session)).thenReturn(snapshot);
        when(stateService.snapshotSuccessfulSkills(session)).thenReturn(evidenceSnapshot);
        when(coordinator.execute(eq("child.llm.skill"), org.mockito.ArgumentMatchers.anyString(), org.mockito.ArgumentMatchers.anyMap(), eq(session), eq(null)))
                .thenReturn("child result");

        Object result = router.execute(capability, Map.of("topic", "mars"), session, null);

        assertThat(result).isEqualTo("child result");
        verify(stateService).restorePlan(session, snapshot);
        verify(stateService).restoreSuccessfulSkills(session, evidenceSnapshot);
        verify(refResolver, never()).resolveArguments(org.mockito.ArgumentMatchers.any(), eq(session));
    }

    @Test
    void nestedYamlDelegationStartsWithFreshEvidenceAndRestoresParentEvidenceAfterward() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = new com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService(
                java.time.Clock.fixed(java.time.Instant.parse("2026-03-15T12:00:00Z"), java.time.ZoneOffset.UTC));
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:child",
                "child.llm.skill",
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child"),
                null);
        CapabilityRegistry capabilityRegistry = mock(CapabilityRegistry.class);
        when(capabilityRegistry.getCapability("child.llm.skill")).thenReturn(capability);

        com.lokiscale.loomspan.internal.skill.YamlSkillManifest manifest = new com.lokiscale.loomspan.internal.skill.YamlSkillManifest();
        manifest.setName("child.llm.skill");
        manifest.setDescription("child.llm.skill");
        manifest.setModel("gpt-5");
        com.lokiscale.loomspan.internal.skill.YamlSkillDefinition definition = new com.lokiscale.loomspan.internal.skill.YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium"));
        com.lokiscale.loomspan.internal.skill.YamlSkillCatalog catalog = mock(com.lokiscale.loomspan.internal.skill.YamlSkillCatalog.class);
        when(catalog.getSkill("child.llm.skill")).thenReturn(definition);

        com.lokiscale.loomspan.internal.model.ModelInteractionFactory chatClientFactory =
                (ignored, mode) -> request -> com.lokiscale.loomspan.internal.model.ModelInteractionResult.content("unused");

        com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine engine = (session, skillDefinition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            assertThat(session.getSuccessfulDirectSkills()).isEmpty();
            session.addSuccessfulDirectSkill("expense_match_search");
            return "child result";
        };
        ExecutionCoordinator coordinator = new ExecutionCoordinator(
                catalog,
                capabilityRegistry,
                chatClientFactory,
                (skillName, session, authentication) -> java.util.List.of(),
                (session, skillDefinition, capabilities, authentication) -> java.util.List.of(),
                engine,
                engine,
                stateService,
                new DefaultAccessGuard());
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory(Map.of("executionCoordinator", coordinator));
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                beanFactory.getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        session.addSuccessfulDirectSkill("parsed_invoice");
        ExecutionFrame parentFrame = stateService.openMissionFrame(session, "parent.visible.skill", Map.of("objective", "parent"));

        Object result = router.execute(capability, Map.of("topic", "mars"), session, null);

        assertThat(result).isEqualTo("child result");
        assertThat(session.getSuccessfulDirectSkills()).containsExactly("parsed_invoice");
        stateService.closeMissionFrame(session, parentFrame);
    }

    @Test
    void nestedYamlDelegationPassesCanonicalMissionInputWithoutSerializingItIntoObjective() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        ExecutionCoordinator coordinator = mock(ExecutionCoordinator.class);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory(Map.of("executionCoordinator", coordinator));
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                beanFactory.getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:child",
                "child.llm.skill",
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child"),
                new SkillInputContractResolver().resolveFromToolSchema("""
                        {
                          "type": "object",
                          "properties": {
                            "invoiceId": { "type": "string" }
                          },
                          "required": ["invoiceId"],
                          "additionalProperties": false
                        }
                        """),
                null);
        PlanSnapshot snapshot = PlanSnapshot.of(null);
        com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot evidenceSnapshot = com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot.of(java.util.Set.of());

        when(stateService.snapshotPlan(session)).thenReturn(snapshot);
        when(stateService.snapshotSuccessfulSkills(session)).thenReturn(evidenceSnapshot);
        when(coordinator.execute(eq("child.llm.skill"), eq("Execute YAML skill 'child.llm.skill' using the provided mission input object."),
                eq(Map.of("invoiceId", "INV-7")), eq(session), eq(null)))
                .thenReturn("child result");

        Object result = router.execute(capability, Map.of("invoiceId", "INV-7"), session, null);

        assertThat(result).isEqualTo("child result");
        verify(refResolver, never()).resolveArguments(org.mockito.ArgumentMatchers.any(), eq(session));
    }

    @Test
    void mappedYamlCapabilityAcceptsDirectRefBackedObjectsOnRootInvocationPath() {
        RefResolver refResolver = mock(RefResolver.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                new StaticListableBeanFactory().getBeanProvider(ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        ByteArrayResource payload = new ByteArrayResource(new byte[]{1, 2, 3});
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:binaryTool",
                "binaryTool",
                "binary tool",
                SkillExecutionDescriptor.none(),
                java.util.Set.of(),
                arguments -> arguments.get("payload"),
                CapabilityKind.YAML_SKILL,
                new CapabilityToolDescriptor("binaryTool", "binary tool", """
                        {
                          "type": "object",
                          "properties": {
                            "payload": {
                              "type": "string",
                              "description": "Provide a ref:// URI for binary content or an inline string value when appropriate.",
                              "x-loomspan-runtime-ref-capable": true
                            }
                          },
                          "required": ["payload"],
                          "additionalProperties": false
                        }
                        """),
                new SkillInputContractResolver().resolveFromToolSchema("""
                        {
                          "type": "object",
                          "properties": {
                            "payload": {
                              "type": "string",
                              "description": "Provide a ref:// URI for binary content or an inline string value when appropriate.",
                              "x-loomspan-runtime-ref-capable": true
                            }
                          },
                          "required": ["payload"],
                          "additionalProperties": false
                        }
                        """),
                "binaryTargetBean#binaryTool");

        when(refResolver.resolveArguments(any(), eq(session))).thenAnswer(invocation -> invocation.getArgument(0));

        Object result = router.execute(capability, Map.of("payload", payload), session, null);

        assertThat(result).isSameAs(payload);
        verify(refResolver).resolveArguments(any(), eq(session));
    }
}
