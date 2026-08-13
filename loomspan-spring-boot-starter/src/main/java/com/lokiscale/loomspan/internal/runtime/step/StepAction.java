package com.lokiscale.loomspan.internal.runtime.step;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import tools.jackson.databind.JsonNode;
import org.springframework.lang.Nullable;

import java.util.Map;

/**
 * The structured action contract the model must return for each step in a plan-step execution loop. The model proposes one action per turn; the runtime validates before executing.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
record StepAction(
        @JsonProperty("stepAction") StepActionType stepAction,
        @JsonProperty("taskId") @Nullable String taskId,
        @JsonProperty("toolName") @Nullable String toolName,
        @JsonProperty("toolArguments") @Nullable Map<String, Object> toolArguments,
        @JsonProperty("finalResponse") @Nullable JsonNode finalResponse)
{
    /**
     * Creates a CALL_TOOL action.
     */
    public static StepAction callTool(String taskId, String toolName, Map<String, Object> toolArguments)
    {
        return new StepAction(StepActionType.CALL_TOOL, taskId, toolName, toolArguments, null);
    }

    /**
     * Creates a FINAL_RESPONSE action.
     */
    public static StepAction finalResponse(JsonNode finalResponse)
    {
        return new StepAction(StepActionType.FINAL_RESPONSE, null, null, null, finalResponse);
    }
}
