package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecision;

public interface SessionUsageService
{
    SessionUsageSnapshot snapshot(LoomspanSession session);

    void recordMissionStart(LoomspanSession session, String skillName);

    void reserveProviderAttempt(LoomspanSession session, String skillName);

    void recordProviderAttemptOutcome(String skillName, ModelExecutionIdentity identity, String outcome,
            ProviderFailureCategory category, ProviderRetryDecision decision);

    void recordModelResponse(LoomspanSession session, String skillName, ModelExecutionIdentity identity, ModelUsageRecord usageRecord);

    void recordToolCall(LoomspanSession session, String skillName, String capabilityName);

    void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome);
}
