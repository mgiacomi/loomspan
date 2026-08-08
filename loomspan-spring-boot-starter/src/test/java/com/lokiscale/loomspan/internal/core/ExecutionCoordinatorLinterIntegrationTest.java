package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver;
import com.lokiscale.loomspan.internal.chat.SkillChatClientFactory;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultToolCallbackFactory;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultToolSurfaceService;
import com.lokiscale.loomspan.internal.runtime.tool.ToolCallbackFactory;
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
        ExecutionCoordinator coordinator = coordinator(catalog, registry, ignored -> chatClient, stateService);
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
                                                    SkillChatClientFactory factory,
                                                    ExecutionStateService stateService) {
        PlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        SkillVisibilityResolver visibilityResolver = (currentSkillName, session, authentication) -> List.of();
        ToolSurfaceService toolSurfaceService = new DefaultToolSurfaceService(visibilityResolver);
        RefResolver refResolver = (value, session) -> value;
        ToolCallbackFactory toolCallbackFactory = new DefaultToolCallbackFactory(
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

    private static final class AdvisedSequenceChatClient implements ChatClient {

        private final List<CallAdvisor> advisors;
        private final List<String> responses;
        private final List<String> userMessagesSeen = new ArrayList<>();
        private final List<String> requestUserMessagesSeen = new ArrayList<>();
        private final List<String> requestSystemMessagesSeen = new ArrayList<>();
        private int callCount;

        private AdvisedSequenceChatClient(List<Advisor> advisors, List<String> responses) {
            this.advisors = advisors.stream().filter(CallAdvisor.class::isInstance).map(CallAdvisor.class::cast).toList();
            this.responses = responses;
        }

        @Override
        public ChatClientRequestSpec prompt() {
            return new RequestSpec();
        }

        @Override
        public ChatClientRequestSpec prompt(String content) {
            return new RequestSpec();
        }

        @Override
        public ChatClientRequestSpec prompt(Prompt prompt) {
            return new RequestSpec();
        }

        @Override
        public Builder mutate() {
            throw new UnsupportedOperationException();
        }

        private final class RequestSpec implements ChatClientRequestSpec {

            private final List<Message> messages = new ArrayList<>();

            @Override
            public Builder mutate() {
                throw new UnsupportedOperationException();
            }

            @Override
            public ChatClientRequestSpec advisors(java.util.function.Consumer<AdvisorSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(Advisor... advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(List<Advisor> advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(Message... messages) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(List<Message> messages) {
                return this;
            }

            @Override
            public <T extends org.springframework.ai.chat.prompt.ChatOptions> ChatClientRequestSpec options(T options) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolNames(String... toolNames) {
                return this;
            }

            @Override
            public ChatClientRequestSpec tools(Object... tools) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(ToolCallback... toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(List<ToolCallback> toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(org.springframework.ai.tool.ToolCallbackProvider... providers) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolContext(Map<String, Object> toolContext) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(String text) {
                messages.add(new SystemMessage(text));
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(java.util.function.Consumer<PromptSystemSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(String text) {
                messages.add(new UserMessage(text));
                userMessagesSeen.add(text);
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(java.util.function.Consumer<PromptUserSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec templateRenderer(org.springframework.ai.template.TemplateRenderer renderer) {
                return this;
            }

            @Override
            public CallResponseSpec call() {
                ChatClientRequest request = new ChatClientRequest(new Prompt(List.copyOf(messages)), Map.of());
                ChatClientResponse response = new AdvisorChainImpl(advisors).nextCall(request);
                return new ResponseSpec(response);
            }

            @Override
            public StreamResponseSpec stream() {
                throw new UnsupportedOperationException();
            }
        }

        private final class AdvisorChainImpl implements CallAdvisorChain {

            private final List<CallAdvisor> advisors;
            private final int index;

            private AdvisorChainImpl(List<CallAdvisor> advisors) {
                this(advisors, 0);
            }

            private AdvisorChainImpl(List<CallAdvisor> advisors, int index) {
                this.advisors = advisors;
                this.index = index;
            }

            @Override
            public ChatClientResponse nextCall(ChatClientRequest chatClientRequest) {
                if (index < advisors.size()) {
                    return advisors.get(index).adviseCall(chatClientRequest, new AdvisorChainImpl(advisors, index + 1));
                }
                requestUserMessagesSeen.add(chatClientRequest.prompt().getUserMessage().getText());
                requestSystemMessagesSeen.add(chatClientRequest.prompt().getSystemMessage().getText());
                String responseText = responses.get(Math.min(callCount, responses.size() - 1));
                callCount++;
                return ChatClientResponse.builder()
                        .chatResponse(new ChatResponse(List.of(new Generation(new AssistantMessage(responseText)))))
                        .build();
            }

            @Override
            public List<CallAdvisor> getCallAdvisors() {
                return advisors;
            }

            @Override
            public CallAdvisorChain copy(CallAdvisor after) {
                int advisorIndex = advisors.indexOf(after);
                if (advisorIndex < 0) {
                    throw new IllegalArgumentException("Advisor not found in chain");
                }
                return new AdvisorChainImpl(advisors, advisorIndex + 1);
            }
        }

        private record ResponseSpec(ChatClientResponse response) implements CallResponseSpec {

            @Override
            public <T> T entity(ParameterizedTypeReference<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> T entity(org.springframework.ai.converter.StructuredOutputConverter<T> converter) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> T entity(Class<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public ChatClientResponse chatClientResponse() {
                return response;
            }

            @Override
            public ChatResponse chatResponse() {
                return response.chatResponse();
            }

            @Override
            public String content() {
                return response.chatResponse().getResult().getOutput().getText();
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<ChatResponse, T> responseEntity(Class<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<ChatResponse, T> responseEntity(ParameterizedTypeReference<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<ChatResponse, T> responseEntity(
                    org.springframework.ai.converter.StructuredOutputConverter<T> converter) {
                throw new UnsupportedOperationException();
            }
        }
    }
}
