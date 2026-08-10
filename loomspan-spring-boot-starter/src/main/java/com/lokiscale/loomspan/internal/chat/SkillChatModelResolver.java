package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;

public interface SkillChatModelResolver
{
    ProviderConnectionRuntime resolve(String skillName, EffectiveSkillExecutionConfiguration configuration);
}
