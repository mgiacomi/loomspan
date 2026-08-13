package com.lokiscale.loomspan.internal.runtime.tool;

import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.util.Map;

public interface CapabilityInvoker
{
    Object invoke(CapabilityMetadata capability,
            Map<String, Object> arguments,
            LoomspanSession session,
            YamlSkillDefinition definition,
            @Nullable Authentication authentication,
            @Nullable String linkedTaskId);
}
