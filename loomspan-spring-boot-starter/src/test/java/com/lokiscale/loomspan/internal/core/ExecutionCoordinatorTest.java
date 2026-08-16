package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.model.ModelInteractionFactory;
import com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException;
import com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultCapabilityInvoker;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultToolSurfaceService;
import com.lokiscale.loomspan.internal.runtime.tool.CapabilityBindingFactory;
import com.lokiscale.loomspan.internal.runtime.tool.ToolSurfaceService;
import com.lokiscale.loomspan.internal.security.AccessGuard;
import com.lokiscale.loomspan.internal.security.DefaultAccessGuard;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.SkillVisibilityResolver;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.lang.Nullable;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.AuthorityUtils;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ForkJoinPool;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.assertj.core.api.Assertions.catchThrowable;

class ExecutionCoordinatorTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void deniesRestrictedRootSkillBeforePlanningOrModelExecution() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of("ROLE_ROOT"),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-root-rbac",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", true))),
                "unused",
                "{\"value\":\"hello\"}");

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                new RecordingModelInteractionFactory(chatClient),
                (value, session) -> value,
                null,
                true);

        LoomspanSession mismatched = new LoomspanSession("session-mismatch", "another.entry", 3);
        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", mismatched, null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessage("Top-level capability name does not match session entry skill");
        assertThat(mismatched.getFramesSnapshot()).isEmpty();

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", session, null))
                .isInstanceOf(AccessDeniedException.class)
                .hasMessageContaining("rootVisibleSkill");
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(session.getJournalSnapshot()).isEmpty();
        assertThat(chatClient.systemMessagesSeen).isEmpty();
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void clearsStaleSessionAuthenticationBeforeUnauthenticatedRootExecution() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of("ROLE_ROOT"),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-root-rbac",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", true))),
                "unused",
                "{\"value\":\"hello\"}");

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                new RecordingModelInteractionFactory(chatClient),
                (value, session) -> value,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);
        session.setAuthentication(UsernamePasswordAuthenticationToken.authenticated(
                "user",
                "pw",
                AuthorityUtils.createAuthorityList("ROLE_ROOT")));

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", session, null))
                .isInstanceOf(AccessDeniedException.class)
                .hasMessageContaining("rootVisibleSkill");
        assertThat(session.getAuthentication()).isEmpty();
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(session.getJournalSnapshot()).isEmpty();
        assertThat(chatClient.systemMessagesSeen).isEmpty();
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void usesValidatedYamlExecutionConfigAndUpdatesPlanThroughToolInvocation() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-1",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", false))),
                "mission complete",
                "{\"value\":\"ref://artifacts/input.txt\"}");
        RecordingModelInteractionFactory factory = new RecordingModelInteractionFactory(chatClient);
        SkillVisibilityResolver visibilityResolver = (currentSkillName, sessionState, authentication) -> List.of(childMetadata);
        RefResolver refResolver = (value, session) -> value instanceof String text && text.startsWith("ref://")
                ? "resolved-content"
                : value;
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);
        String response = coordinator.execute(
                "rootVisibleSkill",
                "Say hello",
                session,
                UsernamePasswordAuthenticationToken.authenticated(
                        "user",
                        "pw",
                        AuthorityUtils.createAuthorityList("ROLE_ALLOWED")));

        assertThat(response).isEqualTo("mission complete");
        assertThat(factory.lastDefinition).isEqualTo(catalog.getSkill("rootVisibleSkill"));
        assertThat(factory.lastDefinition.executionConfiguration()).isEqualTo(executionConfiguration);
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.COMPLETED);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .contains(JournalEntryType.PLAN_CREATED, JournalEntryType.PLAN_UPDATED, JournalEntryType.TOOL_CALL, JournalEntryType.TOOL_RESULT);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsSubsequence(
                        JournalEntryType.PLAN_CREATED,
                        JournalEntryType.TOOL_CALL,
                        JournalEntryType.PLAN_UPDATED,
                        JournalEntryType.TOOL_RESULT);
        assertThat(chatClient.toolNamesSeen).containsExactly("allowedVisibleSkill");
        assertThat(chatClient.toolNamesByCall).containsExactly(List.of(), List.of("allowedVisibleSkill"));
        assertThat(chatClient.systemMessagesSeen).hasSize(2);
        assertThat(chatClient.systemMessagesSeen.get(1)).contains(
                session.getExecutionPlan().orElseThrow().planId(),
                "VALID",
                "task-1",
                "Use allowedVisibleSkill");
        assertThat(chatClient.systemMessagesSeen.get(1)).doesNotContain("plan-1");
        assertThat(chatClient.lastToolResult).isEqualTo("child:resolved-content");
        assertThat(session.getJournalSnapshot().stream()
                .filter(entry -> entry.type() == JournalEntryType.TOOL_CALL)
                .findFirst()
                .orElseThrow()
                .payload()
                .get("details")
                .get("arguments"))
                .isEqualTo(new tools.jackson.databind.ObjectMapper().valueToTree(Map.of("value", "ref://artifacts/input.txt")));
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void skipsPlanningPromptWhenSkillDisablesPlanningMode() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        manifest.setPlanningMode(false);
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "unused-plan",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", true))),
                "mission complete",
                "{\"value\":\"hello\"}",
                false);
        RecordingModelInteractionFactory factory = new RecordingModelInteractionFactory(chatClient);

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                factory,
                (value, session) -> value,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);
        String response = coordinator.execute("rootVisibleSkill", "Say hello", session, null);

        assertThat(response).isEqualTo("mission complete");
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.UNPLANNED_TOOL_EXECUTION, JournalEntryType.TOOL_RESULT);
        assertThat(chatClient.systemMessagesSeen).containsExactly("Execute the mission using only the visible YAML tools when needed.");
        assertThat(chatClient.toolNamesByCall).containsExactly(List.of("allowedVisibleSkill"));
    }

    @Test
    void usesStepExecutionChatClientWhenStepLoopEngineIsSelected() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of());
        manifest.setPlanningMode(true);
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        FakeCoordinatorChatClient defaultChatClient = new FakeCoordinatorChatClient(null, "unused", null, false);
        FakeCoordinatorChatClient stepChatClient = new FakeCoordinatorChatClient(null, "unused", null, false);
        RecordingModelInteractionFactory factory = new RecordingModelInteractionFactory(defaultChatClient, stepChatClient);
        ExecutionStateService stateService = fixedStateService();
        MissionExecutionEngine defaultEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            throw new AssertionError("Default engine should not be selected");
        };
        MissionExecutionEngine stepEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            assertThat(chatClient).isSameAs(stepChatClient);
            return "step loop complete";
        };

        ExecutionCoordinator coordinator = new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                new DefaultToolSurfaceService((currentSkillName, sessionState, authentication) -> List.of()),
                (session, definition, capabilities, authentication) -> List.of(),
                defaultEngine,
                stepEngine,
                stateService,
                new DefaultAccessGuard());

        String response = coordinator.execute("rootVisibleSkill", "Say hello", new LoomspanSession("session-1", "rootVisibleSkill", 3), null);

        assertThat(response).isEqualTo("step loop complete");
        assertThat(factory.stepExecutionRequested).isTrue();
    }

    @Test
    void doesNotSelectStepLoopWhenPlanningModeIsNotExplicitlyEnabled() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of());
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        FakeCoordinatorChatClient defaultChatClient = new FakeCoordinatorChatClient(null, "unused", null, false);
        FakeCoordinatorChatClient stepChatClient = new FakeCoordinatorChatClient(null, "unused", null, false);
        RecordingModelInteractionFactory factory = new RecordingModelInteractionFactory(defaultChatClient, stepChatClient);
        ExecutionStateService stateService = fixedStateService();
        MissionExecutionEngine defaultEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            assertThat(chatClient).isSameAs(defaultChatClient);
            return "one shot complete";
        };
        MissionExecutionEngine stepEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            throw new AssertionError("Step loop should not be selected without explicit planning_mode: true");
        };

        ExecutionCoordinator coordinator = new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                new DefaultToolSurfaceService((currentSkillName, sessionState, authentication) -> List.of()),
                (session, definition, capabilities, authentication) -> List.of(),
                defaultEngine,
                stepEngine,
                stateService,
                new DefaultAccessGuard());

        String response = coordinator.execute("rootVisibleSkill", "Say hello", new LoomspanSession("session-1", "rootVisibleSkill", 3), null);

        assertThat(response).isEqualTo("one shot complete");
        assertThat(factory.stepExecutionRequested).isFalse();
    }

    @Test
    void clearsInheritedEvidenceBeforeNestedSkillExecution() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of());
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        ExecutionStateService stateService = fixedStateService();
        MissionExecutionEngine assertingEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            assertThat(session.getSuccessfulDirectSkills()).isEmpty();
            return "nested complete";
        };
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(),
                new RecordingModelInteractionFactory(new FakeCoordinatorChatClient(null, "unused", null, false)),
                (value, session) -> value,
                null,
                stateService,
                fixedPlanningService(stateService),
                assertingEngine,
                null);

        LoomspanSession session = new LoomspanSession("session-nested", "top.entry", 3);
        ExecutionFrame parentFrame = stateService.openMissionFrame(session, "parent.visible.skill", Map.of("objective", "parent"));
        stateService.recordSuccessfulSkill(session, "invoiceParser", "task-1", false);

        String response = coordinator.execute("rootVisibleSkill", "child objective", session, null);

        assertThat(response).isEqualTo("nested complete");
        assertThat(session.entrySkill()).isEqualTo("top.entry");
        assertThat(session.getSuccessfulDirectSkills()).isEmpty();
        stateService.closeMissionFrame(session, parentFrame);
    }

    @Test
    void failsFastWhenStepLoopFactoryDoesNotImplementStepExecutionClientCreation() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of());
        manifest.setPlanningMode(true);
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        ModelInteractionFactory factory = new ModelInteractionFactory() {
            @Override
            public com.lokiscale.loomspan.internal.model.ModelInteraction create(YamlSkillDefinition definition,
                    com.lokiscale.loomspan.internal.model.ModelInteractionMode mode) {
                throw new UnsupportedOperationException("step execution unavailable");
            }
        };

        ExecutionCoordinator coordinator = new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                new DefaultToolSurfaceService((currentSkillName, sessionState, authentication) -> List.of()),
                (session, definition, capabilities, authentication) -> List.of(),
                (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> "unused",
                (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> "unused",
                fixedStateService(),
                new DefaultAccessGuard());

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", new LoomspanSession("session-1", "rootVisibleSkill", 3), null))
                .isInstanceOf(UnsupportedOperationException.class)
                .hasMessageContaining("step execution unavailable");
    }

    @Test
    void marksTopLevelTraceErroredWhenMissionExecutionThrows() throws Exception {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest("rootVisibleSkill", List.of()),
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        MissionExecutionEngine failingMissionExecutionEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            throw new IllegalStateException("boom");
        };
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(),
                new RecordingModelInteractionFactory(new FakeCoordinatorChatClient(null, "unused", null)),
                (value, session) -> value,
                null,
                failingMissionExecutionEngine);
        LoomspanSession session = new LoomspanSession("session-top-level-failure", "rootVisibleSkill", 3);

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", session, null))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("boom");

        assertThat(session.getExecutionTrace().errored()).isTrue();
        assertThat(session.getExecutionTrace().completed()).isTrue();
        java.nio.file.Path tracePath = session.getExecutionTrace().tracePath();
        try {
            assertThat(tracePath).isNotNull();
            assertThat(java.nio.file.Files.exists(tracePath)).isTrue();
        }
        finally {
            if (tracePath != null) {
                java.nio.file.Files.deleteIfExists(tracePath);
            }
        }
    }

    @Test
    void recordsTopLevelMissionFrameClosureStatusInTrace() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest("rootVisibleSkill", List.of()),
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(),
                new RecordingModelInteractionFactory(new FakeCoordinatorChatClient(null, "mission complete", null, false)),
                (value, session) -> value,
                null);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3, null, TracePersistencePolicy.ALWAYS);
        String response = coordinator.execute("rootVisibleSkill", "Say hello", session, null);

        assertThat(response).isEqualTo("mission complete");
        TraceRecord rootFrameClosed = readTraceRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                .filter(record -> record.frameType() == TraceFrameType.ROOT_MISSION)
                .findFirst()
                .orElseThrow();

        assertThat(rootFrameClosed.metadata()).containsEntry("status", "completed");
    }

    @Test
    void redactsDeclaredAttachmentInputBeforeOpeningMissionFrame() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                attachmentManifest("rootVisibleSkill"),
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(),
                new RecordingModelInteractionFactory(new FakeCoordinatorChatClient(null, "unused", null, false)),
                (value, session) -> value,
                null);
        LoomspanSession session = new LoomspanSession("session-attachment-redaction", "rootVisibleSkill", 3);

        assertThatThrownBy(() -> coordinator.execute(
                "rootVisibleSkill",
                "Parse image",
                Map.of("image", "data:image/jpeg;base64,SECRET_IMAGE_BYTES"),
                session,
                null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Attachment field 'image'");

        TraceRecord rootFrameOpened = readTraceRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_OPENED)
                .filter(record -> record.frameType() == TraceFrameType.ROOT_MISSION)
                .findFirst()
                .orElseThrow();
        assertThat(String.valueOf(rootFrameOpened.data()))
                .contains("\"attachment\":true")
                .contains("\"source\":\"redacted\"")
                .doesNotContain("SECRET_IMAGE_BYTES")
                .doesNotContain("data:image/jpeg");
    }

    @Test
    void preservesMissionFailureWhenCleanupAlsoFails() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest("rootVisibleSkill", List.of()),
                executionConfiguration));
        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        ExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK) {
            @Override
            public void closeFrame(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata) {
                throw new IllegalStateException("cleanup-close");
            }

            @Override
            public void finalizeTrace(LoomspanSession session, TraceCompletion completion) {
                throw new IllegalStateException("cleanup-finalize");
            }
        };
        MissionExecutionEngine failingMissionExecutionEngine = (session, definition, objective, missionInput, chatClient, visibleTools, planningEnabled, authentication) -> {
            throw new IllegalStateException("mission-failed");
        };
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(),
                new RecordingModelInteractionFactory(new FakeCoordinatorChatClient(null, "unused", null)),
                (value, session) -> value,
                null,
                stateService,
                fixedPlanningService(stateService),
                failingMissionExecutionEngine,
                null);
        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);

        Throwable thrown = catchThrowable(() ->
                coordinator.execute("rootVisibleSkill", "Say hello", session, null));

        assertThat(thrown)
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("mission-failed");
        assertThat(thrown.getSuppressed())
                .hasSize(1);
        assertThat(thrown.getSuppressed()[0])
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("cleanup-close");
        assertThat(thrown.getSuppressed()[0].getSuppressed())
                .hasSize(1);
        assertThat(thrown.getSuppressed()[0].getSuppressed()[0])
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("cleanup-finalize");
        assertThat(readTraceRecords(session)).extracting(TraceRecord::recordType)
                .contains(TraceRecordType.ERROR_RECORDED);
    }

    @Test
    void clearsStalePlanBeforeStartingPlanningDisabledMission() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        manifest.setPlanningMode(false);
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "unused-plan",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", false))),
                "mission complete",
                "{\"value\":\"hello\"}",
                false);

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                new RecordingModelInteractionFactory(chatClient),
                (value, session) -> value,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);
        session.replaceExecutionPlan(new ExecutionPlan(
                "stale-plan",
                "old.skill",
                Instant.parse("2026-03-14T12:00:00Z"),
                List.of(new PlanTask("old-task", "Old work", PlanTaskStatus.IN_PROGRESS, "stale note"))));

        String response = coordinator.execute("rootVisibleSkill", "Say hello", session, null);

        assertThat(response).isEqualTo("mission complete");
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(chatClient.systemMessagesSeen).containsExactly("Execute the mission using only the visible YAML tools when needed.");
        assertThat(chatClient.systemMessagesSeen).noneMatch(message -> message.contains("stale-plan"));
    }

    @Test
    void marksMatchingTaskBlockedAndJournalsErrorWhenToolInvocationFails() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                args -> {
                    throw new IllegalStateException("tool exploded");
                },
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-2",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", false))),
                "unused",
                "{\"value\":\"ref://artifacts/input.txt\"}");
        RecordingModelInteractionFactory factory = new RecordingModelInteractionFactory(chatClient);
        SkillVisibilityResolver visibilityResolver = (currentSkillName, sessionState, authentication) -> List.of(childMetadata);
        RefResolver refResolver = (value, session) -> value instanceof String text && text.startsWith("ref://")
                ? "resolved-content"
                : value;
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", session, null))
                .isInstanceOf(IllegalStateException.class)
                .hasRootCauseMessage("tool exploded");
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.BLOCKED);
        assertThat(session.getExecutionPlan().orElseThrow().status()).isEqualTo(PlanStatus.STALE);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .contains(JournalEntryType.PLAN_CREATED, JournalEntryType.PLAN_UPDATED, JournalEntryType.TOOL_CALL, JournalEntryType.ERROR);
    }

    @Test
    void routesUnmappedYamlSkillsBackThroughCoordinatorAndRestoresParentPlan() {
        EffectiveSkillExecutionConfiguration rootExecutionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        EffectiveSkillExecutionConfiguration childExecutionConfiguration = new EffectiveSkillExecutionConfiguration(
                "claude-sonnet",
                "test-connection", AiDriver.ANTHROPIC,
                "anthropic/claude-sonnet-4",
                "medium");
        YamlSkillManifest rootManifest = plannedManifest("rootVisibleSkill", List.of("child.llm.skill"));
        YamlSkillManifest childManifest = plannedManifest("child.llm.skill", List.of("mars.analyzer"));
        rootManifest.setPrompt("PARENT_PROMPT_SENTINEL");
        childManifest.setPrompt("CHILD_PROMPT_SENTINEL");
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(
                new YamlSkillDefinition(new ByteArrayResource(new byte[0]), rootManifest,
                        rootExecutionConfiguration),
                new YamlSkillDefinition(new ByteArrayResource(new byte[0]), childManifest,
                        childExecutionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(rootExecutionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child-llm",
                "child.llm.skill",
                "child llm",
                SkillExecutionDescriptor.from(childExecutionConfiguration),
                java.util.Set.of(),
                arguments -> {
                    throw new UnsupportedOperationException("should route through coordinator");
                },
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child llm"),
                null);
        CapabilityMetadata marsAnalyzerMetadata = new CapabilityMetadata(
                "yaml:mars-analyzer",
                "mars.analyzer",
                "mars analyzer",
                SkillExecutionDescriptor.from(childExecutionConfiguration),
                java.util.Set.of(),
                arguments -> "analysis:" + arguments.get("topic"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("mars.analyzer", "mars analyzer"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);
        registry.register(marsAnalyzerMetadata.name(), marsAnalyzerMetadata);

        FakeCoordinatorChatClient rootChatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-root",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use child.llm.skill", "child.llm.skill", false))),
                "root mission complete",
                "{\"topic\":\"ref://artifacts/topic.txt\"}");
        FakeCoordinatorChatClient childChatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-child",
                        "child.llm.skill",
                        Instant.parse("2026-03-15T12:01:00Z"),
                        List.of(toolTask("child-task-1", "Use mars.analyzer", "mars.analyzer", false))),
                "child mission complete",
                "{\"topic\":\"mars\"}");
        MultiClientModelInteractionFactory factory = new MultiClientModelInteractionFactory(
                java.util.Map.of(
                        rootExecutionConfiguration.frameworkModel(), rootChatClient,
                        childExecutionConfiguration.frameworkModel(), childChatClient));
        SkillVisibilityResolver visibilityResolver = (currentSkillName, sessionState, authentication) ->
                "rootVisibleSkill".equals(currentSkillName)
                        ? List.of(childMetadata)
                        : "child.llm.skill".equals(currentSkillName) ? List.of(marsAnalyzerMetadata) : List.of();
        RefResolver refResolver = (value, session) -> value instanceof String text && text.startsWith("ref://")
                ? "resolved-content"
                : value;

        ExecutionCoordinator rootCoordinator = coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                null,
                true);
        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                rootCoordinator,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 4);
        String response = coordinator.execute("rootVisibleSkill", "Say hello", session, null);

        assertThat(response).isEqualTo("root mission complete");
        assertThat(factory.seenDefinitions).containsExactly(
                catalog.getSkill("rootVisibleSkill"),
                catalog.getSkill("child.llm.skill"));
        assertThat(factory.seenDefinitions).extracting(YamlSkillDefinition::executionConfiguration)
                .containsExactly(rootExecutionConfiguration, childExecutionConfiguration);
        assertThat(rootChatClient.lastToolResult).isEqualTo("child mission complete");
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().planId()).isNotBlank().isNotEqualTo("plan-root");
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.COMPLETED);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsSubsequence(
                        JournalEntryType.PLAN_CREATED,
                        JournalEntryType.TOOL_CALL,
                        JournalEntryType.PLAN_UPDATED,
                        JournalEntryType.TOOL_RESULT);
        assertThat(childChatClient.userMessagesSeen).hasSize(2);
        assertThat(childChatClient.userMessagesSeen.get(1))
                .contains("ref://")
                .doesNotContain("resolved-content");
        assertThat(rootChatClient.systemMessagesSeen).isNotEmpty();
        assertThat(rootChatClient.systemMessagesSeen).allSatisfy(systemPrompt -> assertThat(systemPrompt)
                .contains("PARENT_PROMPT_SENTINEL")
                .doesNotContain("CHILD_PROMPT_SENTINEL"));
        assertThat(childChatClient.systemMessagesSeen).isNotEmpty();
        assertThat(childChatClient.systemMessagesSeen).allSatisfy(systemPrompt -> assertThat(systemPrompt)
                .contains("CHILD_PROMPT_SENTINEL")
                .doesNotContain("PARENT_PROMPT_SENTINEL"));
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void deniesRestrictedToolInvocationAtExecutionTimeWhenAuthenticationLacksRole() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of("ROLE_ALLOWED"),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-rbac",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use allowedVisibleSkill", "allowedVisibleSkill", false))),
                "unused",
                "{\"value\":\"hello\"}");

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                new RecordingModelInteractionFactory(chatClient),
                (value, session) -> value,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);

        assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Say hello", session, null))
                .isInstanceOf(org.springframework.security.access.AccessDeniedException.class)
                .hasMessageContaining("Access denied");
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.BLOCKED);
        assertThat(session.getExecutionPlan().orElseThrow().status()).isEqualTo(PlanStatus.STALE);
    }

    @Test
    void authorizesProtectedChildYamlSkillFromSessionFallback() {
        EffectiveSkillExecutionConfiguration rootExecutionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        EffectiveSkillExecutionConfiguration childExecutionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5-mini",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5-mini",
                "low");
        YamlSkillManifest rootManifest = plannedManifest("rootVisibleSkill", List.of("child.llm.skill"));
        YamlSkillManifest childManifest = plannedManifest("child.llm.skill", List.of("mars.analyzer"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(
                new YamlSkillDefinition(new ByteArrayResource(new byte[0]), rootManifest,
                        rootExecutionConfiguration),
                new YamlSkillDefinition(new ByteArrayResource(new byte[0]), childManifest,
                        childExecutionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(rootExecutionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child-llm",
                "child.llm.skill",
                "child llm",
                SkillExecutionDescriptor.from(childExecutionConfiguration),
                java.util.Set.of("ROLE_ALLOWED"),
                arguments -> {
                    throw new UnsupportedOperationException("should route through coordinator");
                },
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("child.llm.skill", "child llm"),
                null);
        CapabilityMetadata marsAnalyzerMetadata = new CapabilityMetadata(
                "yaml:mars-analyzer",
                "mars.analyzer",
                "mars analyzer",
                SkillExecutionDescriptor.from(childExecutionConfiguration),
                java.util.Set.of("ROLE_ALLOWED"),
                arguments -> "analysis:" + arguments.get("topic"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("mars.analyzer", "mars analyzer"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);
        registry.register(marsAnalyzerMetadata.name(), marsAnalyzerMetadata);

        FakeCoordinatorChatClient rootChatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-root",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(toolTask("task-1", "Use child.llm.skill", "child.llm.skill", false))),
                "root mission complete",
                "{\"topic\":\"mars\"}");
        FakeCoordinatorChatClient childChatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-child",
                        "child.llm.skill",
                        Instant.parse("2026-03-15T12:01:00Z"),
                        List.of(toolTask("child-task-1", "Use mars.analyzer", "mars.analyzer", false))),
                "child mission complete",
                "{\"topic\":\"mars\"}");
        MultiClientModelInteractionFactory factory = new MultiClientModelInteractionFactory(
                java.util.Map.of(
                        rootExecutionConfiguration.frameworkModel(), rootChatClient,
                        childExecutionConfiguration.frameworkModel(), childChatClient));
        SkillVisibilityResolver visibilityResolver = (currentSkillName, sessionState, authentication) ->
                "rootVisibleSkill".equals(currentSkillName)
                        ? List.of(childMetadata)
                        : "child.llm.skill".equals(currentSkillName) ? List.of(marsAnalyzerMetadata) : List.of();

        ExecutionStateService stateService = fixedStateService();
        PlanningService planningService = fixedPlanningService(stateService);
        ToolSurfaceService toolSurfaceService = new DefaultToolSurfaceService(visibilityResolver);
        ExecutionCoordinator[] coordinatorHolder = new ExecutionCoordinator[1];
        CapabilityBindingFactory toolCallbackFactory = new DefaultCapabilityInvoker(
                new CapabilityExecutionRouter(
                        (value, session) -> value,
                        coordinatorProvider(() -> coordinatorHolder[0]),
                        stateService,
                        new DefaultAccessGuard()),
                planningService,
                stateService);
        MissionExecutionEngine missionExecutionEngine = missionExecutionEngine(planningService, stateService);
        coordinatorHolder[0] = new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                toolSurfaceService,
                toolCallbackFactory,
                missionExecutionEngine,
                missionExecutionEngine,
                stateService,
                new DefaultAccessGuard());
        ExecutionCoordinator coordinator = coordinatorHolder[0];

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 4);
        String response = coordinator.execute(
                "rootVisibleSkill",
                "Say hello",
                session,
                UsernamePasswordAuthenticationToken.authenticated(
                        "user",
                        "pw",
                        AuthorityUtils.createAuthorityList("ROLE_ALLOWED")));

        assertThat(response).isEqualTo("root mission complete");
        assertThat(rootChatClient.lastToolResult).isEqualTo("child mission complete");
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.COMPLETED);
        assertThat(session.getAuthentication()).isPresent();
    }

    @Test
    void closesMissionFramesWhenMissionExecutionTimesOut() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = manifest("rootVisibleSkill", List.of());
        manifest.setPlanningMode(false);
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            ExecutionStateService stateService = fixedStateService();
            PlanningService planningService = fixedPlanningService(stateService);
            MissionExecutionEngine missionExecutionEngine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofMillis(25),
                    missionExecutor);
            ExecutionCoordinator coordinator = coordinator(
                    catalog,
                    registry,
                    (currentSkillName, sessionState, authentication) -> List.of(),
                    (ignored, mode) -> new BlockingCoordinatorChatClient(),
                    (value, session) -> value,
                    null,
                    stateService,
                    planningService,
                    missionExecutionEngine,
                    null);

            LoomspanSession session = new LoomspanSession("session-timeout", "rootVisibleSkill", 3);

            assertThatThrownBy(() -> coordinator.execute("rootVisibleSkill", "Wait forever", session, null))
                    .isInstanceOf(LoomspanMissionTimeoutException.class)
                    .hasMessageContaining("session-timeout")
                    .hasMessageContaining("rootVisibleSkill");
            assertThat(session.getFramesSnapshot()).isEmpty();
            assertThat(session.getExecutionPlan()).isEmpty();

            List<TraceRecord> traceRecords = new ArrayList<>();
            session.readTraceRecords(traceRecords::add);
            TraceRecord completion = traceRecords.getLast();
            assertThat(completion.recordType()).isEqualTo(TraceRecordType.TRACE_COMPLETED);
            assertThat(completion.metadata()).containsEntry("outcome", TraceOutcome.ABORTED.name());
            String terminalFailureId = (String) completion.metadata().get("terminalFailureId");
            assertThat(terminalFailureId).isNotBlank();
            assertThat(traceRecords.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                    .map(record -> record.metadata().get("failureId")))
                    .contains(terminalFailureId);
        }
    }

    @Test
    void usesMappedSkillToolSchemaInsteadOfGenericMapSchema() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        String methodSchema = "{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}}}";
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                new CapabilityToolDescriptor("allowedVisibleSkill", "child", methodSchema),
                "targetBean#deterministicTarget");

        ExecutionStateService stateService = fixedStateService();
        PlanningService planningService = fixedPlanningService(stateService);
        com.lokiscale.loomspan.internal.runtime.tool.BoundCapability callback = toolCallbackFactory((value, session) -> value, null, stateService, planningService)
                .bind(
                        new LoomspanSession("session-1", "rootVisibleSkill", 2),
                        new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest("rootVisibleSkill", List.of()), executionConfiguration),
                        List.of(childMetadata),
                        null)
                .getFirst();

        assertThat(callback.inputSchema()).isEqualTo(methodSchema);
    }

    @Test
    void journalsUnplannedExecutionWithoutMutatingAmbiguousTasks() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = plannedManifest("rootVisibleSkill", List.of("allowedVisibleSkill"));
        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                executionConfiguration));

        CapabilityMetadata rootMetadata = new CapabilityMetadata(
                "yaml:root",
                "rootVisibleSkill",
                "root",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "root",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("rootVisibleSkill", "root"),
                null);
        CapabilityMetadata childMetadata = new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");

        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register(rootMetadata.name(), rootMetadata);
        registry.register(childMetadata.name(), childMetadata);

        FakeCoordinatorChatClient chatClient = new FakeCoordinatorChatClient(
                new ExecutionPlan(
                        "plan-3",
                        "rootVisibleSkill",
                        Instant.parse("2026-03-15T12:00:00Z"),
                        List.of(
                                toolTask("task-1", "Use allowedVisibleSkill for source A", "allowedVisibleSkill", false),
                                toolTask("task-2", "Use allowedVisibleSkill for source B", "allowedVisibleSkill", false))),
                "mission complete",
                "{\"value\":\"hello\"}");

        ExecutionCoordinator coordinator = coordinator(
                catalog,
                registry,
                (currentSkillName, sessionState, authentication) -> List.of(childMetadata),
                new RecordingModelInteractionFactory(chatClient),
                (value, session) -> value,
                null,
                true);

        LoomspanSession session = new LoomspanSession("session-1", "rootVisibleSkill", 3);
        String response = coordinator.execute("rootVisibleSkill", "Say hello", session, null);

        assertThat(response).isEqualTo("mission complete");
        assertThat(session.getExecutionPlan()).isPresent();
        assertThat(session.getExecutionPlan().orElseThrow().tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.PENDING, PlanTaskStatus.PENDING);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(
                        JournalEntryType.PLAN_CREATED,
                        JournalEntryType.UNPLANNED_TOOL_EXECUTION,
                        JournalEntryType.TOOL_RESULT);
    }

    private static YamlSkillManifest manifest(String name, List<String> allowedSkills) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName(name);
        manifest.setDescription(name);
        manifest.setModel("gpt-5");
        manifest.setAllowedSkills(allowedSkills);
        return manifest;
    }

    private static YamlSkillManifest plannedManifest(String name, List<String> allowedSkills) {
        YamlSkillManifest manifest = manifest(name, allowedSkills);
        manifest.setPlanningMode(true);
        return manifest;
    }

    private static YamlSkillManifest attachmentManifest(String name) {
        YamlSkillManifest manifest = manifest(name, List.of());
        manifest.setPlanningMode(false);
        YamlSkillManifest.InputSchemaManifest root = new YamlSkillManifest.InputSchemaManifest();
        root.setType("object");
        root.setRequired(List.of("image"));
        root.setAdditionalProperties(false);
        YamlSkillManifest.InputSchemaManifest image = new YamlSkillManifest.InputSchemaManifest();
        image.setType("attachment");
        image.setMediaType("image");
        image.setAllowedContentTypes(List.of("image/jpeg"));
        root.setProperties(Map.of("image", image));
        manifest.setInputSchema(root);
        return manifest;
    }

    private static ExecutionCoordinator coordinator(StubYamlSkillCatalog catalog,
                                                    InMemoryCapabilityRegistry registry,
                                                    SkillVisibilityResolver visibilityResolver,
                                                    ModelInteractionFactory factory,
                                                    RefResolver refResolver,
                                                    ExecutionCoordinator routedCoordinator) {
        return coordinator(catalog, registry, visibilityResolver, factory, refResolver, routedCoordinator, (Boolean) null);
    }

    private static ExecutionCoordinator coordinator(StubYamlSkillCatalog catalog,
                                                    InMemoryCapabilityRegistry registry,
                                                    SkillVisibilityResolver visibilityResolver,
                                                    ModelInteractionFactory factory,
                                                    RefResolver refResolver,
                                                    ExecutionCoordinator routedCoordinator,
                                                    @Nullable Boolean dropInvocationAuthenticationForCallbacks) {
        ExecutionStateService stateService = fixedStateService();
        PlanningService planningService = fixedPlanningService(stateService);
        MissionExecutionEngine missionExecutionEngine = missionExecutionEngine(planningService, stateService);
        return coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                routedCoordinator,
                stateService,
                planningService,
                missionExecutionEngine,
                dropInvocationAuthenticationForCallbacks);
    }

    private static ExecutionCoordinator coordinator(StubYamlSkillCatalog catalog,
                                                    InMemoryCapabilityRegistry registry,
                                                    SkillVisibilityResolver visibilityResolver,
                                                    ModelInteractionFactory factory,
                                                    RefResolver refResolver,
                                                    ExecutionCoordinator routedCoordinator,
                                                    MissionExecutionEngine missionExecutionEngine) {
        ExecutionStateService stateService = fixedStateService();
        PlanningService planningService = fixedPlanningService(stateService);
        return coordinator(
                catalog,
                registry,
                visibilityResolver,
                factory,
                refResolver,
                routedCoordinator,
                stateService,
                planningService,
                missionExecutionEngine,
                null);
    }

    private static ExecutionCoordinator coordinator(StubYamlSkillCatalog catalog,
                                                    InMemoryCapabilityRegistry registry,
                                                    SkillVisibilityResolver visibilityResolver,
                                                    ModelInteractionFactory factory,
                                                    RefResolver refResolver,
                                                    ExecutionCoordinator routedCoordinator,
                                                    ExecutionStateService stateService,
                                                    PlanningService planningService,
                                                    MissionExecutionEngine missionExecutionEngine,
                                                    @Nullable Boolean dropInvocationAuthenticationForCallbacks) {
        ToolSurfaceService toolSurfaceService = new DefaultToolSurfaceService(visibilityResolver);
        CapabilityBindingFactory toolCallbackFactory = toolCallbackFactory(
                refResolver,
                routedCoordinator,
                stateService,
                planningService,
                Boolean.TRUE.equals(dropInvocationAuthenticationForCallbacks));
        AccessGuard accessGuard = new DefaultAccessGuard();
        return new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                toolSurfaceService,
                toolCallbackFactory,
                missionExecutionEngine,
                missionExecutionEngine,
                stateService,
                accessGuard);
    }

    private static CapabilityBindingFactory toolCallbackFactory(RefResolver refResolver,
                                                           ExecutionCoordinator coordinator,
                                                           ExecutionStateService stateService,
                                                           PlanningService planningService) {
        return toolCallbackFactory(refResolver, coordinator, stateService, planningService, false);
    }

    private static CapabilityBindingFactory toolCallbackFactory(RefResolver refResolver,
                                                           ExecutionCoordinator coordinator,
                                                           ExecutionStateService stateService,
                                                           PlanningService planningService,
                                                           boolean dropInvocationAuthenticationForCallbacks) {
        StaticListableBeanFactory beanFactory = coordinator == null
                ? new StaticListableBeanFactory()
                : new StaticListableBeanFactory(java.util.Map.of("executionCoordinator", coordinator));
        DefaultCapabilityInvoker delegate = new DefaultCapabilityInvoker(
                new CapabilityExecutionRouter(
                        refResolver,
                        beanFactory.getBeanProvider(ExecutionCoordinator.class),
                        stateService,
                        new DefaultAccessGuard()),
                planningService,
                stateService);
        if (!dropInvocationAuthenticationForCallbacks) {
            return delegate;
        }
        return (session, definition, capabilities, authentication) ->
                delegate.bind(session, definition, capabilities, null);
    }

    private static ExecutionStateService fixedStateService() {
        return new DefaultExecutionStateService(FIXED_CLOCK);
    }

    private static PlanningService fixedPlanningService(ExecutionStateService stateService) {
        return new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
    }

    private static MissionExecutionEngine missionExecutionEngine(PlanningService planningService,
                                                                 ExecutionStateService stateService) {
        return missionExecutionEngine(planningService, stateService, Duration.ofSeconds(5));
    }

    private static MissionExecutionEngine missionExecutionEngine(PlanningService planningService,
                                                                 ExecutionStateService stateService,
                                                                 Duration timeout) {
        return new DefaultMissionExecutionEngine(
                planningService,
                stateService,
                timeout,
                ForkJoinPool.commonPool());
    }

    private static ObjectProvider<ExecutionCoordinator> coordinatorProvider(java.util.function.Supplier<ExecutionCoordinator> supplier) {
        return new ObjectProvider<>() {
            @Override
            public ExecutionCoordinator getObject(Object... args) {
                return supplier.get();
            }

            @Override
            public ExecutionCoordinator getObject() {
                return supplier.get();
            }

            @Override
            public ExecutionCoordinator getIfAvailable() {
                return supplier.get();
            }

            @Override
            public ExecutionCoordinator getIfUnique() {
                return supplier.get();
            }

            @Override
            public Stream<ExecutionCoordinator> stream() {
                return Stream.of(supplier.get());
            }

            @Override
            public Stream<ExecutionCoordinator> orderedStream() {
                return stream();
            }
        };
    }

    private static PlanTask toolTask(String taskId, String title, String capabilityName, boolean autoCompletable) {
        return new PlanTask(
                taskId,
                title,
                PlanTaskStatus.PENDING,
                capabilityName,
                title,
                List.of(),
                List.of(),
                autoCompletable,
                null);
    }

    private static List<TraceRecord> readTraceRecords(LoomspanSession session) {
        java.util.ArrayList<TraceRecord> records = new java.util.ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static final class StubYamlSkillCatalog extends com.lokiscale.loomspan.internal.skill.YamlSkillCatalog {

        private final java.util.Map<String, YamlSkillDefinition> definitions;

        StubYamlSkillCatalog(YamlSkillDefinition... definitions) {
            super(new com.lokiscale.loomspan.autoconfigure.LoomspanProperties(),
                    new com.lokiscale.loomspan.autoconfigure.LoomspanProperties.Skills());
            this.definitions = java.util.Arrays.stream(definitions)
                    .collect(java.util.stream.Collectors.toMap(definition -> definition.manifest().getName(), definition -> definition));
        }

        @Override
        public YamlSkillDefinition getSkill(String name) {
            return definitions.get(name);
        }
    }

    private static final class RecordingModelInteractionFactory implements ModelInteractionFactory {

        private final FakeCoordinatorChatClient chatClient;
        private final FakeCoordinatorChatClient stepChatClient;
        private YamlSkillDefinition lastDefinition;
        private boolean stepExecutionRequested;

        private RecordingModelInteractionFactory(FakeCoordinatorChatClient chatClient) {
            this(chatClient, chatClient);
        }

        private RecordingModelInteractionFactory(FakeCoordinatorChatClient chatClient,
                                                FakeCoordinatorChatClient stepChatClient) {
            this.chatClient = chatClient;
            this.stepChatClient = stepChatClient;
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteraction create(YamlSkillDefinition definition,
                com.lokiscale.loomspan.internal.model.ModelInteractionMode mode) {
            this.lastDefinition = definition;
            this.stepExecutionRequested = mode == com.lokiscale.loomspan.internal.model.ModelInteractionMode.STEP_EXECUTION;
            return stepExecutionRequested ? stepChatClient : chatClient;
        }
    }

    private static final class MultiClientModelInteractionFactory implements ModelInteractionFactory {

        private final java.util.Map<String, FakeCoordinatorChatClient> clientsByModel;
        private final java.util.List<YamlSkillDefinition> seenDefinitions = new java.util.ArrayList<>();

        private MultiClientModelInteractionFactory(java.util.Map<String, FakeCoordinatorChatClient> clientsByModel) {
            this.clientsByModel = clientsByModel;
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteraction create(YamlSkillDefinition definition,
                com.lokiscale.loomspan.internal.model.ModelInteractionMode mode) {
            seenDefinitions.add(definition);
            EffectiveSkillExecutionConfiguration executionConfiguration = definition.executionConfiguration();
            FakeCoordinatorChatClient chatClient = clientsByModel.get(executionConfiguration.frameworkModel());
            if (chatClient == null) {
                throw new IllegalStateException("No chat client configured for " + executionConfiguration.frameworkModel());
            }
            return chatClient;
        }

    }

    private static final class BlockingCoordinatorChatClient extends com.lokiscale.loomspan.internal.runtime.SimpleChatClient {

        private BlockingCoordinatorChatClient() {
            super(null, "unused");
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteractionResult call(
                com.lokiscale.loomspan.internal.model.ModelInteractionRequest request) {
            try {
                new java.util.concurrent.CountDownLatch(1).await();
                throw new AssertionError("Blocking model unexpectedly resumed");
            }
            catch (InterruptedException ex) {
                Thread.currentThread().interrupt();
                throw new IllegalStateException("Blocking model interrupted", ex);
            }
        }
    }
}
