package com.lokiscale.loomspan.internal.provider;

import java.time.Duration;
import java.util.concurrent.ThreadLocalRandom;

public final class ProviderRetryDecider
{
    public ProviderRetryOutcome decide(ProviderRetryPolicy policy, ProviderFailureDetails failure, int attemptNumber)
    {
        boolean retryable = failure.classification() == ProviderFailureClassification.TRANSIENT;
        if (!policy.enabled() || !retryable)
        {
            return ProviderRetryOutcome.doNotRetry();
        }
        if (attemptNumber >= policy.maxAttempts())
        {
            return new ProviderRetryOutcome(ProviderRetryDecision.ATTEMPTS_EXHAUSTED,
                    Duration.ZERO, RetryDelaySource.NONE);
        }
        Duration backoff = jitteredBackoff(policy, attemptNumber);
        if (failure.retryAfter() != null && failure.retryAfter().compareTo(backoff) > 0)
        {
            Duration boundedRetryAfter = failure.retryAfter().compareTo(policy.maxBackoff()) > 0
                    ? policy.maxBackoff() : failure.retryAfter();
            return new ProviderRetryOutcome(ProviderRetryDecision.RETRY,
                    Duration.ofMillis(saturatedMillis(boundedRetryAfter)), RetryDelaySource.RETRY_AFTER);
        }
        return new ProviderRetryOutcome(ProviderRetryDecision.RETRY, backoff, RetryDelaySource.BACKOFF);
    }

    private Duration jitteredBackoff(ProviderRetryPolicy policy, int failedAttemptNumber)
    {
        double factor = Math.pow(policy.multiplier(), Math.max(0, failedAttemptNumber - 1));
        long maximumMillis = saturatedMillis(policy.maxBackoff());
        double millis = Math.min(maximumMillis, saturatedMillis(policy.initialBackoff()) * factor);
        if (!Double.isFinite(millis)) millis = maximumMillis;
        if (policy.jitter() > 0 && millis > 0)
        {
            double adjustment = ThreadLocalRandom.current().nextDouble(-policy.jitter(), policy.jitter());
            millis *= 1.0d + adjustment;
        }
        long bounded = Math.max(0L, Math.min(maximumMillis, Math.round(millis)));
        return Duration.ofMillis(bounded);
    }

    private static long saturatedMillis(Duration duration)
    {
        try { return duration.toMillis(); }
        catch (ArithmeticException overflow) { return Long.MAX_VALUE; }
    }
}
