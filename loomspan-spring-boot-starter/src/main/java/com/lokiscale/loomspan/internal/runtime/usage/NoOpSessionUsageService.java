package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecision;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;

public final class NoOpSessionUsageService implements SessionUsageService
{
    @Override
    public SessionUsageSnapshot snapshot(LoomspanSession session)
    {
        return session == null ? SessionUsageSnapshot.empty() : session.getSessionUsage().orElse(SessionUsageSnapshot.empty());
    }

    @Override
    public void recordMissionStart(LoomspanSession session, String skillName)
    {
    }

    @Override
    public void recordModelResponse(LoomspanSession session, String skillName, ModelExecutionIdentity identity,
            ModelUsageRecord usageRecord)
    {
    }

    @Override
    public void reserveProviderAttempt(LoomspanSession session, String skillName)
    {
    }

    @Override
    public void recordProviderAttemptOutcome(String skillName, ModelExecutionIdentity identity, String outcome,
            ProviderFailureCategory category, ProviderRetryDecision decision)
    {
    }

    @Override
    public void recordToolCall(LoomspanSession session, String skillName, String capabilityName)
    {
    }

    @Override
    public void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome)
    {
    }
}
