package com.lokiscale.loomspan.internal.chat;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.AdvisorTraceRecorder;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidator;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.provider.*;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.anthropic.api.AnthropicApi;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.advisor.api.Advisor;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.ai.chat.prompt.ChatOptions;
import org.springframework.ai.google.genai.GoogleGenAiChatOptions;
import org.springframework.ai.ollama.api.OllamaChatOptions;
import org.springframework.ai.openai.OpenAiChatOptions;
import org.springframework.core.io.ByteArrayResource;

import java.util.List;
import java.util.Map;
import java.time.Clock;
import java.util.regex.Pattern;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class SpringAiSkillChatClientFactoryTests {

    @Test
    void debugLogsDistinguishSameDriverConnectionsWithoutTransportDetails() {
        Logger logger = (Logger) LoggerFactory.getLogger(SpringAiSkillChatClientFactory.class);
        Level previousLevel = logger.getLevel();
        ListAppender<ILoggingEvent> appender = new ListAppender<>();
        appender.start();
        logger.addAppender(appender);
        logger.setLevel(Level.DEBUG);
        try {
            ChatModel first = mock(ChatModel.class);
            ChatModel second = mock(ChatModel.class);
            SkillChatModelResolver resolver = resolver(Map.of("openai-main", first, "openrouter", second));
            captureFactoryInvocation(definition(new EffectiveSkillExecutionConfiguration(
                    "native", "openai-main", AiDriver.OPENAI, "gpt-5", null)),
                    new NoOpSkillAdvisorResolver(), resolver);
            captureFactoryInvocation(definition(new EffectiveSkillExecutionConfiguration(
                    "routed", "openrouter", AiDriver.OPENAI, "anthropic/sonnet", null)),
                    new NoOpSkillAdvisorResolver(), resolver);

            String logs = appender.list.stream().map(ILoggingEvent::getFormattedMessage).toList().toString();
            assertThat(logs)
                    .contains("connection=openai-main", "connection=openrouter", "driver=OPENAI")
                    .doesNotContain("api-key", "Authorization", "base-url", "http://", "https://");
        }
        finally {
            logger.detachAppender(appender);
            logger.setLevel(previousLevel);
            appender.stop();
        }
    }

    @Test
    void selectsChatModelFromResolvedProvider() {
        ChatModel ollamaChatModel = mock(ChatModel.class);
        SkillChatModelResolver chatModelResolver = mock(SkillChatModelResolver.class);
        EffectiveSkillExecutionConfiguration configuration = new EffectiveSkillExecutionConfiguration(
                "ollama-llama3", "test-connection", AiDriver.OLLAMA, "llama3.2", null);
        when(chatModelResolver.resolve("test.skill", configuration)).thenReturn(runtime(ollamaChatModel));

        CapturedFactoryResult result = captureFactoryInvocation(
                definition(configuration),
                new NoOpSkillAdvisorResolver(),
                chatModelResolver);

        assertThat(result.resolvedChatModel()).isSameAs(ollamaChatModel);
        assertThat(result.options()).isInstanceOf(OllamaChatOptions.class);
        verify(chatModelResolver).resolve("test.skill", configuration);
    }

    @Test
    void createsClientWithProviderModelAndNoThinkingOptionWhenThinkingIsNull() {
        FactoryClient created = createFactoryBackedClient(definition(new EffectiveSkillExecutionConfiguration(
                "ollama-llama3",
                "test-connection", AiDriver.OLLAMA,
                "llama3.2",
                null)));

        assertThat(created.options()).isInstanceOf(OllamaChatOptions.class);
        OllamaChatOptions options = (OllamaChatOptions) created.options();
        assertThat(options.getModel()).isEqualTo("llama3.2");
    }

    @Test
    void createsClientWithThinkingOptionWhenEffectiveConfigProvidesIt() {
        CapturedFactoryResult result = captureFactoryInvocation(
                definition(new EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                new NoOpSkillAdvisorResolver(),
                resolver(Map.of("test-connection", mock(ChatModel.class))));

        assertThat(result.options()).isInstanceOf(OpenAiChatOptions.class);
        OpenAiChatOptions options = (OpenAiChatOptions) result.options();
        assertThat(options.getModel()).isEqualTo("openai/gpt-5");
        assertThat(options.getTemperature()).isEqualTo(1.0);
        assertThat(options.getReasoningEffort()).isEqualTo("medium");
    }

    @Test
    void usesDefaultTemperatureForGpt5MiniToOverrideInheritedSpringAiSamplingOptions() {
        CapturedFactoryResult result = captureFactoryInvocation(
                definition(new EffectiveSkillExecutionConfiguration(
                        "openai-gpt-5-mini",
                        "test-connection", AiDriver.OPENAI,
                        "gpt-5-mini",
                        null)),
                new NoOpSkillAdvisorResolver(),
                resolver(Map.of("test-connection", mock(ChatModel.class))));

        assertThat(result.options()).isInstanceOf(OpenAiChatOptions.class);
        OpenAiChatOptions options = (OpenAiChatOptions) result.options();
        assertThat(options.getModel()).isEqualTo("gpt-5-mini");
        assertThat(options.getTemperature()).isEqualTo(1.0);
    }

    @Test
    void createsClientWithResolvedAdvisorsAndProviderOptions() {
        YamlSkillDefinition definition = definition(new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium"));
        Advisor advisor = mock(Advisor.class);

        CapturedFactoryResult result = captureFactoryInvocation(definition, ignored -> List.of(advisor));

        assertThat(result.options()).isInstanceOf(OpenAiChatOptions.class);
        assertThat(result.advisors().getFirst()).isSameAs(advisor);
        assertThat(result.advisors().getLast()).isInstanceOf(ProviderAttemptCallAdvisor.class);
        verify(result.builder()).defaultOptions(any(ChatOptions.class));
        verify(result.builder()).defaultAdvisors(anyList());
        verify(result.builder()).build();
    }

    @Test
    void doesNotAttachAdvisorsWhenResolverReturnsEmptyList() {
        YamlSkillDefinition definition = definition(new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium"));

        CapturedFactoryResult result = captureFactoryInvocation(definition, new NoOpSkillAdvisorResolver());

        assertThat(result.client()).isSameAs(result.factoryClient());
        assertThat(result.advisors()).singleElement().isInstanceOf(ProviderAttemptCallAdvisor.class);
        verify(result.builder()).defaultOptions(any(ChatOptions.class));
        verify(result.builder()).defaultAdvisors(anyList());
    }

    @Test
    void createForStepExecutionOmitsResolvedAdvisors() {
        YamlSkillDefinition definition = definition(new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium"));
        Advisor passthroughAdvisor = mock(Advisor.class);
        RecordingBuilderFactory builderFactory = new RecordingBuilderFactory();
        SpringAiSkillChatClientFactory factory = new SpringAiSkillChatClientFactory(
                resolver(Map.of("test-connection", mock(ChatModel.class))),
                SpringAiSkillChatClientFactory.defaultAdapters(),
                ignored -> List.of(
                        passthroughAdvisor,
                        outputSchemaAdvisor(),
                        linterAdvisor()),
                stateService(),
                new ModelUsageExtractor(),
                new NoOpSessionUsageService(),
                builderFactory);

        ChatClient created = factory.createForStepExecution(definition);
        CapturedFactoryResult result = builderFactory.result(created);

        assertThat(result.advisors().getFirst()).isSameAs(passthroughAdvisor);
        assertThat(result.advisors().getLast()).isInstanceOf(ProviderAttemptCallAdvisor.class);
        verify(result.builder()).defaultOptions(any(ChatOptions.class));
        verify(result.builder()).defaultAdvisors(anyList());
    }

    @Test
    void dispatchesToProviderSpecificAdapter() {
        ChatOptions openAi = createFactoryBackedClient(definition(new EffectiveSkillExecutionConfiguration(
                "gpt-5",
                "test-connection", AiDriver.OPENAI,
                "openai/gpt-5",
                "medium"))).options();
        assertThat(openAi).isInstanceOf(OpenAiChatOptions.class);
        assertThat(((OpenAiChatOptions) openAi).getReasoningEffort()).isEqualTo("medium");

        ChatOptions anthropic = createFactoryBackedClient(definition(new EffectiveSkillExecutionConfiguration(
                "claude-sonnet",
                "test-connection", AiDriver.ANTHROPIC,
                "anthropic/claude-sonnet-4",
                "medium"))).options();
        assertThat(anthropic).isInstanceOf(AnthropicChatOptions.class);
        assertThat(((AnthropicChatOptions) anthropic).getThinking().type()).isEqualTo(AnthropicApi.ThinkingType.ENABLED);
        assertThat(((AnthropicChatOptions) anthropic).getThinking().budgetTokens()).isEqualTo(4096);

        ChatOptions gemini = createFactoryBackedClient(definition(new EffectiveSkillExecutionConfiguration(
                "gemini-pro",
                "test-connection", AiDriver.GEMINI,
                "google/gemini-2.5-pro",
                "medium"))).options();
        assertThat(gemini).isInstanceOf(GoogleGenAiChatOptions.class);
        assertThat(((GoogleGenAiChatOptions) gemini).getIncludeThoughts()).isTrue();
        assertThat(((GoogleGenAiChatOptions) gemini).getThinkingBudget()).isEqualTo(4096);

        ChatOptions ollama = createFactoryBackedClient(definition(new EffectiveSkillExecutionConfiguration(
                "ollama-llama3",
                "test-connection", AiDriver.OLLAMA,
                "llama3.2",
                null))).options();
        assertThat(ollama).isInstanceOf(OllamaChatOptions.class);
        assertThat(((OllamaChatOptions) ollama).getModel()).isEqualTo("llama3.2");
    }

    @Test
    void throwsExecutionTimeErrorForUnavailableProvider() {
        SpringAiSkillChatClientFactory factory = new SpringAiSkillChatClientFactory(
                new DefaultSkillChatModelResolver(Map.of("openai-main", runtime(mock(ChatModel.class)))),
                SpringAiSkillChatClientFactory.defaultAdapters(),
                new NoOpSkillAdvisorResolver(),
                stateService(),
                new ModelUsageExtractor(),
                new NoOpSessionUsageService());

        assertThatThrownBy(() -> factory.create(definition(new EffectiveSkillExecutionConfiguration(
                "ollama-llama3",
                "test-connection", AiDriver.OLLAMA,
                "llama3.2",
                null))))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("test-connection")
                .hasMessageContaining("OLLAMA")
                .hasMessageContaining("test.skill");
    }

    private FactoryClient createFactoryBackedClient(YamlSkillDefinition definition) {
        return captureFactoryInvocation(definition, new NoOpSkillAdvisorResolver()).factoryClient();
    }

    private CapturedFactoryResult captureFactoryInvocation(YamlSkillDefinition definition, SkillAdvisorResolver skillAdvisorResolver) {
        EffectiveSkillExecutionConfiguration executionConfiguration = definition.executionConfiguration();
        return captureFactoryInvocation(
                definition,
                skillAdvisorResolver,
                resolver(Map.of(executionConfiguration.connection(), mock(ChatModel.class))));
    }

    private CapturedFactoryResult captureFactoryInvocation(YamlSkillDefinition definition,
                                                            SkillAdvisorResolver skillAdvisorResolver,
                                                            SkillChatModelResolver chatModelResolver) {
        RecordingBuilderFactory builderFactory = new RecordingBuilderFactory();
        SpringAiSkillChatClientFactory factory = new SpringAiSkillChatClientFactory(
                chatModelResolver,
                SpringAiSkillChatClientFactory.defaultAdapters(),
                skillAdvisorResolver,
                stateService(),
                new ModelUsageExtractor(),
                new NoOpSessionUsageService(),
                builderFactory);

        ChatClient created = factory.create(definition);
        return builderFactory.result(created);
    }

    private SkillChatModelResolver resolver(Map<String, ChatModel> modelsByConnection) {
        Map<String, ProviderConnectionRuntime> runtimes = new java.util.LinkedHashMap<>();
        modelsByConnection.forEach((name, model) -> runtimes.put(name, runtime(model)));
        return new DefaultSkillChatModelResolver(runtimes);
    }

    private static ProviderConnectionRuntime runtime(ChatModel model) {
        LoomspanProperties.ProviderRetryProperties retry = new LoomspanProperties.ProviderRetryProperties();
        retry.setEnabled(false);
        return new ProviderConnectionRuntime(model, AiDriver.OPENAI, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                ProviderRetryPolicy.from(retry), ignored -> ProviderFailureDetails.unknown());
    }

    private static DefaultExecutionStateService stateService() {
        return new DefaultExecutionStateService(Clock.systemUTC());
    }

    private YamlSkillDefinition definition(EffectiveSkillExecutionConfiguration configuration) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("test.skill");
        manifest.setDescription("test.skill");
        manifest.setModel(configuration.frameworkModel());
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, configuration);
    }

    private OutputSchemaCallAdvisor outputSchemaAdvisor() {
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        return new OutputSchemaCallAdvisor(
                "test.skill",
                schema,
                new OutputSchemaValidator(),
                new com.lokiscale.loomspan.internal.outputschema.OutputSchemaPromptAugmentor(),
                1,
                outcome -> { },
                AdvisorTraceRecorder.noOp());
    }

    private LinterCallAdvisor linterAdvisor() {
        return new LinterCallAdvisor(
                "test.skill",
                "regex",
                Pattern.compile("^ok$"),
                "must match",
                1,
                outcome -> { },
                AdvisorTraceRecorder.noOp());
    }

    private record CapturedFactoryResult(ChatClient client,
                                          FactoryClient factoryClient,
                                          ChatOptions options,
                                          ChatClient.Builder builder,
                                          ChatModel resolvedChatModel) {

        List<Advisor> advisors() {
            return factoryClient.advisors();
        }
    }

    private static final class RecordingBuilderFactory implements SpringAiSkillChatClientFactory.ChatClientBuilderFactory {

        private ChatModel resolvedChatModel;
        private ChatClient.Builder builder;
        private FactoryClient factoryClient;

        @Override
        public ChatClient.Builder create(ChatModel chatModel) {
            this.resolvedChatModel = chatModel;
            this.builder = mock(ChatClient.Builder.class);
            this.factoryClient = new FactoryClient();
            when(builder.defaultOptions(any())).thenAnswer(invocation -> {
                factoryClient.setOptions(invocation.getArgument(0));
                return builder;
            });
            when(builder.defaultAdvisors(anyList())).thenAnswer(invocation -> {
                factoryClient.setAdvisors(List.copyOf(invocation.getArgument(0)));
                return builder;
            });
            when(builder.build()).thenReturn(factoryClient);
            return builder;
        }

        private CapturedFactoryResult result(ChatClient created) {
            return new CapturedFactoryResult(created, factoryClient, factoryClient.options(), builder, resolvedChatModel);
        }
    }

    private static final class FactoryClient implements ChatClient {

        private ChatOptions options;
        private List<Advisor> advisors = List.of();

        void setOptions(ChatOptions options) {
            this.options = options;
        }

        void setAdvisors(List<Advisor> advisors) {
            this.advisors = advisors;
        }

        ChatOptions options() {
            return options;
        }

        List<Advisor> advisors() {
            return advisors;
        }

        @Override
        public ChatClientRequestSpec prompt() {
            throw new UnsupportedOperationException();
        }

        @Override
        public ChatClientRequestSpec prompt(String content) {
            throw new UnsupportedOperationException();
        }

        @Override
        public ChatClientRequestSpec prompt(org.springframework.ai.chat.prompt.Prompt prompt) {
            throw new UnsupportedOperationException();
        }

        @Override
        public Builder mutate() {
            throw new UnsupportedOperationException();
        }
    }
}
