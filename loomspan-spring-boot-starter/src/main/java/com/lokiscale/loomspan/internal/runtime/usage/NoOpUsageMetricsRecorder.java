package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecision;

public final class NoOpUsageMetricsRecorder implements UsageMetricsRecorder
{
    @Override
    public void recordSkillInvocation(String skillName)
    {
    }

    @Override
    public void recordModelUsage(String skillName, ModelExecutionIdentity identity, ModelUsageRecord usageRecord)
    {
    }

    @Override
    public void recordProviderAttempt(String skillName, ModelExecutionIdentity identity, String outcome,
            ProviderFailureCategory category, ProviderRetryDecision decision)
    {
    }

    @Override
    public void recordToolInvocation(String skillName, String toolName, String outcome)
    {
    }

    @Override
    public void recordToolAccuracy(String skillName, String linterType, String outcome)
    {
    }

    @Override
    public void recordLinterOutcome(LinterOutcome outcome)
    {
    }

    @Override
    public void recordGuardrailTrip(String skillName, GuardrailType guardrailType)
    {
    }
}
