package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver;
import com.lokiscale.loomspan.internal.model.ModelInteractionFactory;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
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
import com.lokiscale.loomspan.internal.security.DefaultAccessGuard;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.SkillVisibilityResolver;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.Advisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.messages.Message;
import org.springframework.ai.chat.messages.SystemMessage;
import org.springframework.ai.chat.messages.UserMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.core.io.ByteArrayResource;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ForkJoinPool;

import static org.assertj.core.api.Assertions.assertThat;

class ExecutionCoordinatorLinterIntegrationTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void retriesLintedYamlSkillThroughAdvisorAndRecordsOutcome() {
        EffectiveSkillExecutionConfiguration executionConfiguration = new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium");
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("lintedSkill");
        manifest.setDescription("lintedSkill");
        manifest.setModel("gpt-5");
        manifest.setAllowedSkills(List.of());
        manifest.setPlanningMode(false);
        YamlSkillManifest.RegexManifest regex = new YamlSkillManifest.RegexManifest();
        regex.setPattern("^OK:.*$");
        regex.setMessage("Start with OK:");
        YamlSkillManifest.LinterManifest linter = new YamlSkillManifest.LinterManifest();
        linter.setType("regex");
        linter.setMaxRetries(2);
        linter.setRegex(regex);
        manifest.setLinter(linter);
        YamlSkillDefinition definition = new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, executionConfiguration);

        StubYamlSkillCatalog catalog = new StubYamlSkillCatalog(definition);
        InMemoryCapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityMetadata metadata = new CapabilityMetadata(
                "yaml:linted",
                "lintedSkill",
                "lintedSkill",
                SkillExecutionDescriptor.from(executionConfiguration),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("lintedSkill", "lintedSkill"),
                null);
        registry.register(metadata.name(), metadata);

        ExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        AdvisedSequenceChatClient chatClient = new AdvisedSequenceChatClient(
                new DefaultSkillAdvisorResolver(stateService).resolve(definition),
                List.of("not ok", "OK: corrected"));
        ExecutionCoordinator coordinator = coordinator(catalog, registry,
                (ignored, mode) -> chatClient, stateService);
        LoomspanSession session = new LoomspanSession("session-1", "lintedSkill", 3);

        String response = coordinator.execute("lintedSkill", "Produce YAML", session, null);

        assertThat(response).isEqualTo("OK: corrected");
        assertThat(chatClient.callCount).isEqualTo(2);
        assertThat(chatClient.requestUserMessagesSeen.get(1))
                .isEqualTo("Produce YAML");
        assertThat(chatClient.requestSystemMessagesSeen.get(1))
                .contains("Linter validation failed")
                .contains("Start with OK:");
        assertThat(session.getLastLinterOutcome()).isPresent();
        assertThat(session.getLastLinterOutcome().orElseThrow())
                .extracting(com.lokiscale.loomspan.internal.linter.LinterOutcome::status,
                        com.lokiscale.loomspan.internal.linter.LinterOutcome::retryCount)
                .containsExactly(LinterOutcomeStatus.PASSED, 1);
        assertThat(session.getJournalSnapshot())
                .extracting(JournalEntry::type)
                .containsOnly(JournalEntryType.LINTER);
        assertThat(session.getJournalSnapshot().stream()
                .filter(entry -> entry.type() == JournalEntryType.LINTER)
                .count()).isEqualTo(2);
    }

    private static ExecutionCoordinator coordinator(StubYamlSkillCatalog catalog,
                                                    InMemoryCapabilityRegistry registry,
                                                    ModelInteractionFactory factory,
                                                    ExecutionStateService stateService) {
        PlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        SkillVisibilityResolver visibilityResolver = (currentSkillName, session, authentication) -> List.of();
        ToolSurfaceService toolSurfaceService = new DefaultToolSurfaceService(visibilityResolver);
        RefResolver refResolver = (value, session) -> value;
        CapabilityBindingFactory toolCallbackFactory = new DefaultCapabilityInvoker(
                new CapabilityExecutionRouter(
                        refResolver,
                        new StaticListableBeanFactory().getBeanProvider(ExecutionCoordinator.class),
                        stateService,
                        new com.lokiscale.loomspan.internal.security.DefaultAccessGuard()),
                planningService,
                stateService);
        MissionExecutionEngine missionExecutionEngine = new DefaultMissionExecutionEngine(
                planningService,
                stateService,
                Duration.ofSeconds(5),
                ForkJoinPool.commonPool());
        return new ExecutionCoordinator(
                catalog,
                registry,
                factory,
                toolSurfaceService,
                toolCallbackFactory,
                missionExecutionEngine,
                missionExecutionEngine,
                stateService,
                new DefaultAccessGuard());
    }

    private static final class StubYamlSkillCatalog extends com.lokiscale.loomspan.internal.skill.YamlSkillCatalog {

        private final Map<String, YamlSkillDefinition> definitions;

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

    private static final class AdvisedSequenceChatClient implements com.lokiscale.loomspan.internal.model.ModelInteraction {

        private final List<String> responses;
        private final List<String> requestUserMessagesSeen = new ArrayList<>();
        private final List<String> requestSystemMessagesSeen = new ArrayList<>();
        private final ChatClient delegate;
        private int callCount;

        private AdvisedSequenceChatClient(List<Advisor> advisors, List<String> responses) {
            this.responses = responses;
            org.springframework.ai.chat.model.ChatModel model = prompt -> {
                requestUserMessagesSeen.add(prompt.getUserMessage().getText());
                requestSystemMessagesSeen.add(prompt.getSystemMessage().getText());
                String responseText = responses.get(Math.min(callCount, responses.size() - 1));
                callCount++;
                return new ChatResponse(List.of(new Generation(new AssistantMessage(responseText))));
            };
            this.delegate = ChatClient.builder(model).defaultAdvisors(advisors).build();
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteractionResult call(
                com.lokiscale.loomspan.internal.model.ModelInteractionRequest request) {
            return new com.lokiscale.loomspan.internal.springai.SpringAiModelInteraction(delegate).call(request);
        }
    }
}