package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.chat.prompt.ChatOptions;
import org.springframework.ai.google.genai.GoogleGenAiChatOptions;
import org.springframework.ai.ollama.api.OllamaChatOptions;
import org.springframework.ai.openai.OpenAiChatOptions;

public final class SpringAiChatOptionsContributor
{
    private static final int LOW = 1024;
    private static final int MEDIUM = 4096;
    private static final int HIGH = 8192;

    public ChatOptions.Builder<?> createOptions(EffectiveSkillExecutionConfiguration configuration)
    {
        return switch (configuration.driver())
        {
            case OPENAI -> openAi(configuration);
            case ANTHROPIC -> anthropic(configuration);
            case GEMINI -> gemini(configuration);
            case OLLAMA -> OllamaChatOptions.builder().model(configuration.providerModel());
        };
    }

    private ChatOptions.Builder<?> openAi(EffectiveSkillExecutionConfiguration configuration)
    {
        OpenAiChatOptions.Builder builder = OpenAiChatOptions.builder().model(configuration.providerModel());
        String model = configuration.providerModel();
        int separator = model.lastIndexOf('/');
        if ((separator >= 0 ? model.substring(separator + 1) : model).startsWith("gpt-5")) builder.temperature(1.0);
        if (configuration.thinkingLevel() != null) builder.reasoningEffort(configuration.thinkingLevel());
        return builder;
    }

    private ChatOptions.Builder<?> anthropic(EffectiveSkillExecutionConfiguration configuration)
    {
        AnthropicChatOptions.Builder builder = AnthropicChatOptions.builder().model(configuration.providerModel());
        if (configuration.thinkingLevel() != null)
        {
            int thinkingBudget = budget(configuration.thinkingLevel());
            int maxTokens = Math.max(AnthropicChatOptions.DEFAULT_MAX_TOKENS, Math.multiplyExact(thinkingBudget, 2));
            builder.maxTokens(maxTokens).thinkingEnabled(thinkingBudget);
        }
        return builder;
    }

    private ChatOptions.Builder<?> gemini(EffectiveSkillExecutionConfiguration configuration)
    {
        GoogleGenAiChatOptions.Builder builder = GoogleGenAiChatOptions.builder().model(configuration.providerModel());
        if (configuration.thinkingLevel() != null)
        {
            builder.includeThoughts(true).thinkingBudget(budget(configuration.thinkingLevel()));
        }
        return builder;
    }

    private int budget(String level)
    {
        return switch (level)
        {
            case "low" -> LOW;
            case "medium" -> MEDIUM;
            case "high" -> HIGH;
            default -> throw new IllegalArgumentException("Unsupported thinking level '" + level + "'");
        };
    }
}
