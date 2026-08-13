package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.google.genai.GoogleGenAiChatOptions;
import org.springframework.ai.ollama.api.OllamaChatOptions;
import org.springframework.ai.openai.OpenAiChatOptions;

import static org.assertj.core.api.Assertions.assertThat;

class SpringAiChatOptionsContributorTest
{
    private final SpringAiChatOptionsContributor contributor = new SpringAiChatOptionsContributor();

    @Test
    void providerOptionContributorsApplyImmutableTargetOptions()
    {
        OpenAiChatOptions firstOpenAi = (OpenAiChatOptions) contributor.createOptions(configuration(
                AiDriver.OPENAI, "openai/gpt-5", "high")).build();
        OpenAiChatOptions secondOpenAi = (OpenAiChatOptions) contributor.createOptions(configuration(
                AiDriver.OPENAI, "openai/gpt-4.1", null)).build();

        assertThat(firstOpenAi.getModel()).isEqualTo("openai/gpt-5");
        assertThat(firstOpenAi.getTemperature()).isEqualTo(1.0);
        assertThat(firstOpenAi.getReasoningEffort()).isEqualTo("high");
        assertThat(secondOpenAi.getModel()).isEqualTo("openai/gpt-4.1");
        assertThat(secondOpenAi.getTemperature()).isNull();
        assertThat(secondOpenAi.getReasoningEffort()).isNull();

        AnthropicChatOptions anthropic = (AnthropicChatOptions) contributor.createOptions(configuration(
                AiDriver.ANTHROPIC, "claude-sonnet-4", "medium")).build();
        assertThat(anthropic.getModel()).isEqualTo("claude-sonnet-4");
        assertThat(anthropic.getThinking()).isNotNull();

        GoogleGenAiChatOptions gemini = (GoogleGenAiChatOptions) contributor.createOptions(configuration(
                AiDriver.GEMINI, "gemini-2.5-pro", "low")).build();
        assertThat(gemini.getModel()).isEqualTo("gemini-2.5-pro");
        assertThat(gemini.getIncludeThoughts()).isTrue();
        assertThat(gemini.getThinkingBudget()).isEqualTo(1024);

        OllamaChatOptions ollama = (OllamaChatOptions) contributor.createOptions(configuration(
                AiDriver.OLLAMA, "qwen3:8b", null)).build();
        assertThat(ollama.getModel()).isEqualTo("qwen3:8b");
    }

    @ParameterizedTest
    @CsvSource({
            "low, 1024, 4096",
            "medium, 4096, 8192",
            "high, 8192, 16384"
    })
    void anthropicRequestOptionsKeepThinkingBudgetBelowMaxTokens(
            String level, long expectedBudget, int expectedMaxTokens)
    {
        AnthropicChatOptions options = (AnthropicChatOptions) contributor.createOptions(configuration(
                AiDriver.ANTHROPIC, "claude-sonnet-4", level)).build();

        assertThat(options.getThinking()).isNotNull();
        assertThat(options.getThinking().asEnabled().budgetTokens()).isEqualTo(expectedBudget);
        assertThat(options.getMaxTokens()).isEqualTo(expectedMaxTokens);
        assertThat(options.getThinking().asEnabled().budgetTokens()).isLessThan(options.getMaxTokens().longValue());
    }

    private EffectiveSkillExecutionConfiguration configuration(AiDriver driver, String model, String thinking)
    {
        return new EffectiveSkillExecutionConfiguration("alias", "connection", driver, model, thinking);
    }
}
