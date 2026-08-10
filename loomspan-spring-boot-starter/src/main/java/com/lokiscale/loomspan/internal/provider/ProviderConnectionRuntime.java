package com.lokiscale.loomspan.internal.provider;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import org.springframework.ai.chat.model.ChatModel;

import java.util.Objects;

public record ProviderConnectionRuntime(ChatModel chatModel, AiDriver driver,
        AttemptOwnership attemptOwnership, ProviderRetryPolicy retryPolicy,
        ProviderFailureTranslator failureTranslator)
{
    public ProviderConnectionRuntime
    {
        Objects.requireNonNull(chatModel, "chatModel must not be null");
        Objects.requireNonNull(driver, "driver must not be null");
        Objects.requireNonNull(attemptOwnership, "attemptOwnership must not be null");
        Objects.requireNonNull(retryPolicy, "retryPolicy must not be null");
        Objects.requireNonNull(failureTranslator, "failureTranslator must not be null");
    }
}
