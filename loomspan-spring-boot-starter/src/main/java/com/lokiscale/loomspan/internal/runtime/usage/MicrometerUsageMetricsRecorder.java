package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecision;
import io.micrometer.core.instrument.MeterRegistry;

import java.util.Locale;
import java.util.Objects;

public class MicrometerUsageMetricsRecorder implements UsageMetricsRecorder
{
    private final MeterRegistry meterRegistry;

    public MicrometerUsageMetricsRecorder(MeterRegistry meterRegistry)
    {
        this.meterRegistry = Objects.requireNonNull(meterRegistry, "meterRegistry must not be null");
    }

    @Override
    public void recordSkillInvocation(String skillName)
    {
        meterRegistry.counter("loomspan.skill.calls", "skill", normalize(skillName)).increment();
    }

    @Override
    public void recordModelUsage(String skillName, ModelExecutionIdentity identity, ModelUsageRecord usageRecord)
    {
        Objects.requireNonNull(usageRecord, "usageRecord must not be null");
        Objects.requireNonNull(identity, "identity must not be null");
        String[] identityTags = {
                "skill", normalize(skillName),
                "connection", normalize(identity.connection()),
                "driver", normalize(identity.driver().name())
        };

        meterRegistry.counter("loomspan.model.calls", append(identityTags, "precision", usageRecord.precision().name())).increment();
        meterRegistry.counter("loomspan.model.prompt.units", identityTags).increment(usageRecord.promptUnits());
        meterRegistry.counter("loomspan.model.completion.units", identityTags).increment(usageRecord.completionUnits());
        meterRegistry.counter("loomspan.model.usage.units", append(identityTags, "precision", usageRecord.precision().name())).increment(usageRecord.totalUnits());
    }

    @Override
    public void recordProviderAttempt(String skillName, ModelExecutionIdentity identity, String outcome,
            ProviderFailureCategory category, ProviderRetryDecision decision)
    {
        Objects.requireNonNull(identity, "identity must not be null");
        meterRegistry.counter("loomspan.provider.attempts",
                "skill", normalize(skillName),
                "model", normalize(identity.frameworkModel()),
                "connection", normalize(identity.connection()),
                "driver", normalize(identity.driver().name()),
                "outcome", normalize(outcome),
                "category", category == null ? "none" : category.name().toLowerCase(Locale.ROOT),
                "decision", decision == null ? "none" : decision.name().toLowerCase(Locale.ROOT)).increment();
    }

    @Override
    public void recordToolInvocation(String skillName, String toolName, String outcome)
    {
        meterRegistry.counter(
                "loomspan.tool.calls",
                "skill", normalize(skillName),
                "tool", normalize(toolName),
                "outcome", normalize(outcome)).increment();
    }

    @Override
    public void recordToolAccuracy(String skillName, String linterType, String outcome)
    {
        meterRegistry.counter(
                "loomspan.tool.accuracy.samples",
                "skill", normalize(skillName),
                "linter", normalize(linterType),
                "outcome", normalize(outcome)).increment();
    }

    @Override
    public void recordLinterOutcome(LinterOutcome outcome)
    {
        Objects.requireNonNull(outcome, "outcome must not be null");
        meterRegistry.counter(
                "loomspan.linter.outcomes",
                "skill", normalize(outcome.skillName()),
                "status", outcome.status().name(),
                "linter", normalize(outcome.linterType())).increment();
    }

    @Override
    public void recordGuardrailTrip(String skillName, GuardrailType guardrailType)
    {
        meterRegistry.counter(
                "loomspan.guardrail.trips",
                "skill", normalize(skillName),
                "guardrail", guardrailType.name(),
                "outcome", "quota_exceeded").increment();
    }

    private String normalize(String value)
    {
        if (value == null || value.isBlank())
        {
            return "unknown";
        }
        return value.toLowerCase(Locale.ROOT);
    }

    private String[] append(String[] tags, String key, String value)
    {
        String[] result = java.util.Arrays.copyOf(tags, tags.length + 2);
        result[tags.length] = key;
        result[tags.length + 1] = value;
        return result;
    }
}
