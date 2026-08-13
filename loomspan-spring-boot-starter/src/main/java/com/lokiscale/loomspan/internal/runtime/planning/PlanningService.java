package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.springframework.lang.Nullable;

import java.util.List;
import java.util.Map;
import java.util.Optional;

public interface PlanningService
{
    Optional<ExecutionPlan> initializePlan(
            LoomspanSession session,
            String objective,
            @Nullable Map<String, Object> missionInput,
            YamlSkillDefinition definition,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools);

    Optional<ExecutionPlan> markToolStarted(LoomspanSession session, CapabilityMetadata capability, Map<String, Object> arguments);

    Optional<ExecutionPlan> markTaskStarted(LoomspanSession session, String taskId, String capabilityName, @Nullable Map<String, Object> arguments);

    Optional<ExecutionPlan> markToolCompleted(LoomspanSession session,
            String taskId,
            String capabilityName,
            @Nullable Object result);

    Optional<ExecutionPlan> markToolFailed(LoomspanSession session, String taskId, String capabilityName, RuntimeException ex);
}
