package com.lokiscale.loomspan.internal.provider;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;

import java.time.Duration;
import java.util.Objects;

public record ProviderRetryPolicy(boolean enabled, int maxAttempts, Duration initialBackoff,
        double multiplier, Duration maxBackoff, double jitter)
{
    public ProviderRetryPolicy
    {
        Objects.requireNonNull(initialBackoff, "initialBackoff must not be null");
        Objects.requireNonNull(maxBackoff, "maxBackoff must not be null");
    }

    public static ProviderRetryPolicy from(LoomspanProperties.ProviderRetryProperties properties)
    {
        Objects.requireNonNull(properties, "properties must not be null");
        return new ProviderRetryPolicy(properties.isEnabled(), properties.effectiveMaxAttempts(),
                properties.getInitialBackoff(), properties.getMultiplier(), properties.getMaxBackoff(),
                properties.getJitter());
    }
}
