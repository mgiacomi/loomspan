package com.lokiscale.loomspan.internal.provider;

import java.time.Duration;

public record ProviderRetryOutcome(ProviderRetryDecision decision, Duration delay, RetryDelaySource delaySource)
{
    public static ProviderRetryOutcome doNotRetry()
    {
        return new ProviderRetryOutcome(ProviderRetryDecision.DO_NOT_RETRY, Duration.ZERO, RetryDelaySource.NONE);
    }
}
