package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContractCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.advisor.api.Advisor;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.ai.chat.prompt.ChatOptions;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.anthropic.api.AnthropicApi;
import org.springframework.ai.google.genai.GoogleGenAiChatOptions;
import org.springframework.ai.ollama.api.OllamaChatOptions;
import org.springframework.ai.openai.OpenAiChatOptions;

import java.util.EnumMap;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;

public class SpringAiSkillChatClientFactory implements SkillChatClientFactory
{
    private static final Logger log = LoggerFactory.getLogger(SpringAiSkillChatClientFactory.class);

    private static final int LOW_THINKING_BUDGET = 1024;
    private static final int MEDIUM_THINKING_BUDGET = 4096;
    private static final int HIGH_THINKING_BUDGET = 8192;

    private final SkillChatModelResolver chatModelResolver;
    private final ChatClientBuilderFactory chatClientBuilderFactory;
    private final Map<AiDriver, SkillChatOptionsAdapter> adaptersByDriver;
    private final SkillAdvisorResolver skillAdvisorResolver;
    private final ExecutionStateService executionStateService;
    private final ModelUsageExtractor modelUsageExtractor;
    private final SessionUsageService sessionUsageService;

    public SpringAiSkillChatClientFactory(SkillChatModelResolver chatModelResolver,
            List<SkillChatOptionsAdapter> adapters,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService)
    {
        this(chatModelResolver, adapters, skillAdvisorResolver, executionStateService,
                modelUsageExtractor, sessionUsageService, ChatClient::builder);
    }

    SpringAiSkillChatClientFactory(SkillChatModelResolver chatModelResolver,
            List<SkillChatOptionsAdapter> adapters,
            SkillAdvisorResolver skillAdvisorResolver,
            ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor,
            SessionUsageService sessionUsageService,
            ChatClientBuilderFactory chatClientBuilderFactory)
    {
        this.chatModelResolver = Objects.requireNonNull(chatModelResolver, "chatModelResolver must not be null");
        Objects.requireNonNull(adapters, "adapters must not be null");
        this.skillAdvisorResolver = Objects.requireNonNull(skillAdvisorResolver, "skillAdvisorResolver must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.modelUsageExtractor = Objects.requireNonNull(modelUsageExtractor, "modelUsageExtractor must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
        this.chatClientBuilderFactory = Objects.requireNonNull(chatClientBuilderFactory, "chatClientBuilderFactory must not be null");
        this.adaptersByDriver = new EnumMap<>(AiDriver.class);
        for (SkillChatOptionsAdapter adapter : adapters)
        {
            this.adaptersByDriver.put(adapter.driver(), adapter);
        }
    }

    @Override
    public ChatClient create(YamlSkillDefinition definition)
    {
        return create(definition, true);
    }

    @Override
    public ChatClient createForStepExecution(YamlSkillDefinition definition)
    {
        return create(definition, false);
    }

    private ChatClient create(YamlSkillDefinition definition, boolean includeFinalResponseValidators)
    {
        Objects.requireNonNull(definition, "definition must not be null");
        EffectiveSkillExecutionConfiguration executionConfiguration = definition.requireExecutionConfiguration();
        String skillName = definition.manifest().getName();
        SkillChatOptionsAdapter adapter = adaptersByDriver.get(executionConfiguration.driver());
        if (adapter == null)
        {
            throw new IllegalStateException("No ChatOptions adapter configured for driver " + executionConfiguration.driver());
        }
        ProviderConnectionRuntime runtime = chatModelResolver.resolve(skillName, executionConfiguration);
        ChatModel chatModel = runtime.chatModel();
        ChatOptions options = adapter.createOptions(executionConfiguration);
        List<Advisor> advisors = resolvedAdvisors(skillAdvisorResolver.resolve(definition), includeFinalResponseValidators,
                new ProviderAttemptCallAdvisor(runtime, executionStateService, modelUsageExtractor, sessionUsageService));
        ChatClient.Builder builder = chatClientBuilderFactory.create(chatModel);
        builder.defaultOptions(options);
        if (!advisors.isEmpty())
        {
            builder.defaultAdvisors(advisors);
        }
        ChatClient delegate = builder.build();
        log.debug(
                "Created skill ChatClient for skill '{}' frameworkModel={} connection={} driver={} chatModelType={} delegateType={} advisors={}",
                skillName,
                executionConfiguration.frameworkModel(),
                executionConfiguration.connection(),
                executionConfiguration.driver(),
                chatModel.getClass().getName(),
                delegate.getClass().getName(),
                includeFinalResponseValidators ? advisorNames(advisors) : advisorNames(advisors) + " (step execution)");
        return delegate;
    }

    private List<Advisor> resolvedAdvisors(List<Advisor> advisors, boolean includeFinalResponseValidators,
            ProviderAttemptCallAdvisor providerAttemptCallAdvisor)
    {
        List<Advisor> resolved = advisors == null ? List.of() : advisors;
        if (!includeFinalResponseValidators)
        {
            resolved = resolved.stream()
                    .filter(advisor -> !(advisor instanceof OutputSchemaCallAdvisor))
                    .filter(advisor -> !(advisor instanceof EvidenceContractCallAdvisor))
                    .filter(advisor -> !(advisor instanceof LinterCallAdvisor))
                    .toList();
        }
        ArrayList<Advisor> instrumented = new ArrayList<>(resolved);
        instrumented.add(providerAttemptCallAdvisor);
        return List.copyOf(instrumented);
    }

    interface ChatClientBuilderFactory
    {

        ChatClient.Builder create(ChatModel chatModel);
    }

    public static List<SkillChatOptionsAdapter> defaultAdapters()
    {
        return List.of(
                new OpenAiOptionsAdapter(),
                new AnthropicOptionsAdapter(),
                new GeminiOptionsAdapter(),
                new OllamaOptionsAdapter());
    }

    private static String advisorNames(List<Advisor> advisors)
    {
        return advisors.stream()
                .map(advisor -> advisor.getName() + ":" + advisor.getClass().getSimpleName())
                .toList()
                .toString();
    }

    private static final class OpenAiOptionsAdapter implements SkillChatOptionsAdapter
    {
        @Override
        public AiDriver driver()
        {
            return AiDriver.OPENAI;
        }

        @Override
        public ChatOptions createOptions(EffectiveSkillExecutionConfiguration executionConfiguration)
        {
            OpenAiChatOptions.Builder builder = OpenAiChatOptions.builder()
                    .model(executionConfiguration.providerModel());
            if (usesGpt5DefaultsOnlySampling(executionConfiguration.providerModel()))
            {
                builder.temperature(1.0);
            }
            if (executionConfiguration.thinkingLevel() != null)
            {
                builder.reasoningEffort(executionConfiguration.thinkingLevel());
            }
            return builder.build();
        }

        private static boolean usesGpt5DefaultsOnlySampling(String providerModel)
        {
            String modelName = providerModel;
            int namespaceSeparator = modelName.lastIndexOf('/');
            if (namespaceSeparator >= 0)
            {
                modelName = modelName.substring(namespaceSeparator + 1);
            }
            return modelName.startsWith("gpt-5");
        }
    }

    private static final class AnthropicOptionsAdapter implements SkillChatOptionsAdapter
    {
        @Override
        public AiDriver driver()
        {
            return AiDriver.ANTHROPIC;
        }

        @Override
        public ChatOptions createOptions(EffectiveSkillExecutionConfiguration executionConfiguration)
        {
            AnthropicChatOptions.Builder builder = AnthropicChatOptions.builder()
                    .model(executionConfiguration.providerModel());
            if (executionConfiguration.thinkingLevel() != null)
            {
                builder.thinking(AnthropicApi.ThinkingType.ENABLED, thinkingBudget(executionConfiguration.thinkingLevel()));
            }
            return builder.build();
        }
    }

    private static final class GeminiOptionsAdapter implements SkillChatOptionsAdapter
    {
        @Override
        public AiDriver driver()
        {
            return AiDriver.GEMINI;
        }

        @Override
        public ChatOptions createOptions(EffectiveSkillExecutionConfiguration executionConfiguration)
        {
            GoogleGenAiChatOptions.Builder builder = GoogleGenAiChatOptions.builder()
                    .model(executionConfiguration.providerModel());
            if (executionConfiguration.thinkingLevel() != null)
            {
                builder.includeThoughts(true)
                        .thinkingBudget(thinkingBudget(executionConfiguration.thinkingLevel()));
            }
            return builder.build();
        }
    }

    private static final class OllamaOptionsAdapter implements SkillChatOptionsAdapter
    {
        @Override
        public AiDriver driver()
        {
            return AiDriver.OLLAMA;
        }

        @Override
        public ChatOptions createOptions(EffectiveSkillExecutionConfiguration executionConfiguration)
        {
            return OllamaChatOptions.builder()
                    .model(executionConfiguration.providerModel())
                    .build();
        }
    }

    private static int thinkingBudget(String thinkingLevel)
    {
        return switch (thinkingLevel)
        {
            case "low" -> LOW_THINKING_BUDGET;
            case "medium" -> MEDIUM_THINKING_BUDGET;
            case "high" -> HIGH_THINKING_BUDGET;
            default -> throw new IllegalArgumentException("Unsupported thinking level '" + thinkingLevel + "'");
        };
    }
}
