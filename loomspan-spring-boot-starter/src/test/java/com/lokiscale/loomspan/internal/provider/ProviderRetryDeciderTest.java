package com.lokiscale.loomspan.internal.provider;

import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class ProviderRetryDeciderTest
{
    private final ProviderRetryDecider decider = new ProviderRetryDecider();
    private final ProviderRetryPolicy policy = new ProviderRetryPolicy(
            true, 3, Duration.ofMillis(100), 2.0d, Duration.ofSeconds(1), 0.0d);

    @Test
    void retriesTransientFailuresWithExponentialBackoff()
    {
        ProviderRetryOutcome first = decider.decide(policy, failure(ProviderFailureClassification.TRANSIENT, null), 1);
        ProviderRetryOutcome second = decider.decide(policy, failure(ProviderFailureClassification.TRANSIENT, null), 2);

        assertThat(first).isEqualTo(new ProviderRetryOutcome(
                ProviderRetryDecision.RETRY, Duration.ofMillis(100), RetryDelaySource.BACKOFF));
        assertThat(second).isEqualTo(new ProviderRetryOutcome(
                ProviderRetryDecision.RETRY, Duration.ofMillis(200), RetryDelaySource.BACKOFF));
    }

    @Test
    void honorsLongerRetryAfterAndStopsAtTheAttemptLimit()
    {
        assertThat(decider.decide(policy, failure(ProviderFailureClassification.TRANSIENT, Duration.ofMillis(750)), 1))
                .isEqualTo(new ProviderRetryOutcome(
                        ProviderRetryDecision.RETRY, Duration.ofMillis(750), RetryDelaySource.RETRY_AFTER));
        assertThat(decider.decide(policy, failure(ProviderFailureClassification.TRANSIENT, null), 3).decision())
                .isEqualTo(ProviderRetryDecision.ATTEMPTS_EXHAUSTED);
    }

    @Test
    void capsRetryAfterAndSaturatesDurationMultiplicationWithoutOverflow()
    {
        assertThat(decider.decide(policy, failure(ProviderFailureClassification.TRANSIENT, Duration.ofDays(1)), 1))
                .isEqualTo(new ProviderRetryOutcome(
                        ProviderRetryDecision.RETRY, Duration.ofSeconds(1), RetryDelaySource.RETRY_AFTER));
        ProviderRetryPolicy huge = new ProviderRetryPolicy(true, 3,
                Duration.ofSeconds(Long.MAX_VALUE), Double.MAX_VALUE,
                Duration.ofSeconds(Long.MAX_VALUE), 0.0d);
        assertThat(decider.decide(huge, failure(ProviderFailureClassification.TRANSIENT, null), 2).delay())
                .isEqualTo(Duration.ofMillis(Long.MAX_VALUE));
        assertThat(decider.decide(huge,
                failure(ProviderFailureClassification.TRANSIENT, Duration.ofSeconds(Long.MAX_VALUE)), 1).delay())
                .isEqualTo(Duration.ofMillis(Long.MAX_VALUE));
    }

    @Test
    void doesNotRetryPermanentUnknownOrDisabledFailures()
    {
        assertThat(decider.decide(policy, failure(ProviderFailureClassification.PERMANENT, null), 1).decision())
                .isEqualTo(ProviderRetryDecision.DO_NOT_RETRY);
        assertThat(decider.decide(policy, failure(ProviderFailureClassification.UNKNOWN, null), 1).decision())
                .isEqualTo(ProviderRetryDecision.DO_NOT_RETRY);
        ProviderRetryPolicy disabled = new ProviderRetryPolicy(
                false, 1, Duration.ofMillis(100), 2.0d, Duration.ofSeconds(1), 0.0d);
        assertThat(decider.decide(disabled, failure(ProviderFailureClassification.TRANSIENT, null), 1).decision())
                .isEqualTo(ProviderRetryDecision.DO_NOT_RETRY);
    }

    private static ProviderFailureDetails failure(
            ProviderFailureClassification classification, Duration retryAfter)
    {
        return new ProviderFailureDetails(classification, ProviderFailureCategory.UNKNOWN,
                null, retryAfter, null, null, null, List.of());
    }
}
