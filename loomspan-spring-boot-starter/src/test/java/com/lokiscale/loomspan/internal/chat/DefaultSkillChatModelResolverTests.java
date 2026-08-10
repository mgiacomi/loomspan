package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.provider.*;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.model.ChatModel;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;

class DefaultSkillChatModelResolverTests {

    @Test
    void resolvesDistinctModelsForTwoConnectionsUsingSameDriver() {
        ChatModel nativeOpenAi = mock(ChatModel.class);
        ChatModel openRouter = mock(ChatModel.class);
        ProviderConnectionRuntime nativeRuntime = runtime(nativeOpenAi);
        ProviderConnectionRuntime routedRuntime = runtime(openRouter);
        DefaultSkillChatModelResolver resolver = new DefaultSkillChatModelResolver(Map.of(
                "openai-main", nativeRuntime,
                "openrouter", routedRuntime));

        EffectiveSkillExecutionConfiguration nativeConfiguration = new EffectiveSkillExecutionConfiguration(
                "fast", "openai-main", AiDriver.OPENAI, "gpt-fast", null);
        EffectiveSkillExecutionConfiguration routedConfiguration = new EffectiveSkillExecutionConfiguration(
                "routed", "openrouter", AiDriver.OPENAI, "anthropic/sonnet", null);

        assertThat(resolver.resolve("fastSkill", nativeConfiguration)).isSameAs(nativeRuntime);
        assertThat(resolver.resolve("routedSkill", routedConfiguration)).isSameAs(routedRuntime);
    }

    @Test
    void reusesOneConnectionModelForAliasesWithDifferentProviderModelIds() {
        ChatModel shared = mock(ChatModel.class);
        ProviderConnectionRuntime sharedRuntime = runtime(shared);
        DefaultSkillChatModelResolver resolver = new DefaultSkillChatModelResolver(Map.of("openai-main", sharedRuntime));

        assertThat(resolver.resolve("fastSkill", new EffectiveSkillExecutionConfiguration(
                "fast", "openai-main", AiDriver.OPENAI, "gpt-fast", null))).isSameAs(sharedRuntime);
        assertThat(resolver.resolve("deepSkill", new EffectiveSkillExecutionConfiguration(
                "deep", "openai-main", AiDriver.OPENAI, "gpt-deep", "high"))).isSameAs(sharedRuntime);
    }

    @Test
    void nestedChildResolutionUsesTheChildConnectionRatherThanTheParentConnection() {
        ChatModel parent = mock(ChatModel.class);
        ChatModel child = mock(ChatModel.class);
        ProviderConnectionRuntime parentRuntime = runtime(parent);
        ProviderConnectionRuntime childRuntime = runtime(child);
        DefaultSkillChatModelResolver resolver = new DefaultSkillChatModelResolver(Map.of(
                "parent-connection", parentRuntime, "child-connection", childRuntime));
        EffectiveSkillExecutionConfiguration parentConfiguration = new EffectiveSkillExecutionConfiguration(
                "parent-model", "parent-connection", AiDriver.OPENAI, "gpt-parent", null);
        EffectiveSkillExecutionConfiguration childConfiguration = new EffectiveSkillExecutionConfiguration(
                "child-model", "child-connection", AiDriver.OPENAI, "gpt-child", null);

        assertThat(resolver.resolve("parentSkill", parentConfiguration)).isSameAs(parentRuntime);
        assertThat(resolver.resolve("nestedChildSkill", childConfiguration)).isSameAs(childRuntime).isNotSameAs(parentRuntime);
    }

    @Test
    void failsClearlyWhenConnectionIsUnavailable() {
        DefaultSkillChatModelResolver resolver = new DefaultSkillChatModelResolver(Map.of(
                "openai-main", runtime(mock(ChatModel.class))));
        EffectiveSkillExecutionConfiguration configuration = new EffectiveSkillExecutionConfiguration(
                "local", "ollama-east", AiDriver.OLLAMA, "qwen3", null);

        assertThatThrownBy(() -> resolver.resolve("invoiceParser", configuration))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("invoiceParser")
                .hasMessageContaining("local")
                .hasMessageContaining("ollama-east")
                .hasMessageContaining("OLLAMA");
    }

    private static ProviderConnectionRuntime runtime(ChatModel model) {
        return new ProviderConnectionRuntime(model, AiDriver.OPENAI, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                ProviderRetryPolicy.from(new LoomspanProperties.ProviderRetryProperties()), ignored -> ProviderFailureDetails.unknown());
    }
}
