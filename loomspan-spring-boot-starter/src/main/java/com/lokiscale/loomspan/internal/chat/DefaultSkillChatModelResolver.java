package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;

import java.util.Map;
import java.util.Objects;

public class DefaultSkillChatModelResolver implements SkillChatModelResolver
{
    private final Map<String, ProviderConnectionRuntime> modelsByConnection;

    public DefaultSkillChatModelResolver(Map<String, ProviderConnectionRuntime> modelsByConnection)
    {
        Objects.requireNonNull(modelsByConnection, "modelsByConnection must not be null");
        this.modelsByConnection = Map.copyOf(modelsByConnection);
    }

    @Override
    public ProviderConnectionRuntime resolve(String skillName, EffectiveSkillExecutionConfiguration configuration)
    {
        Objects.requireNonNull(skillName, "skillName must not be null");
        Objects.requireNonNull(configuration, "configuration must not be null");
        ProviderConnectionRuntime chatModel = modelsByConnection.get(configuration.connection());

        if (chatModel == null)
        {
            throw new IllegalStateException("No ChatModel configured for connection '" + configuration.connection()
                    + "' (driver " + configuration.driver() + ", framework model '" + configuration.frameworkModel()
                    + "') required by skill '" + skillName + "'");
        }
        return chatModel;
    }
}
