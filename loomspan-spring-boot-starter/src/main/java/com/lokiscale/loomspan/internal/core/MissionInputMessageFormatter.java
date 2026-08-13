package com.lokiscale.loomspan.internal.core;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import org.springframework.lang.Nullable;

import java.util.Map;

public final class MissionInputMessageFormatter
{
    private static final ObjectMapper OBJECT_MAPPER =
            com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().applicationConversion();

    private MissionInputMessageFormatter()
    {
    }

    public static String buildMissionContext(String objective, @Nullable Map<String, Object> missionInput, @Nullable String capabilityName)
    {
        String normalizedObjective = sanitizeObjective(objective, capabilityName);
        if (missionInput == null || missionInput.isEmpty())
        {
            return normalizedObjective;
        }

        return """
                %s

                Canonical mission input:
                %s
                """.formatted(
                normalizedObjective == null || normalizedObjective.isBlank() ? "Use the provided mission inputs." : normalizedObjective,
                prettyPrint(missionInput));
    }

    public static String buildUserMessage(String objective, @Nullable Map<String, Object> missionInput)
    {
        if (missionInput == null || missionInput.isEmpty())
        {
            return objective;
        }

        return """
                Mission objective:
                %s

                Canonical mission input:
                %s

                Use the canonical mission input object as the source of truth for structured fields.
                """.formatted(
                objective == null || objective.isBlank() ? "(none)" : objective,
                prettyPrint(missionInput));
    }

    private static String sanitizeObjective(String objective, @Nullable String capabilityName)
    {
        if (objective == null || objective.isBlank())
        {
            return "(none)";
        }

        String sanitized = objective;
        if (capabilityName != null && !capabilityName.isBlank())
        {
            String exactPrefix = "Execute YAML skill '" + capabilityName + "' using the provided mission input object.";
            if (sanitized.startsWith(exactPrefix))
            {
                return "Use the provided mission inputs.";
            }
            sanitized = sanitized.replace("YAML skill '" + capabilityName + "'", "the mission");
            sanitized = sanitized.replace("'" + capabilityName + "'", "the mission");
            sanitized = sanitized.replace(capabilityName, "the mission");
        }

        return sanitized;
    }

    private static String prettyPrint(Map<String, Object> missionInput)
    {
        try
        {
            return OBJECT_MAPPER.writerWithDefaultPrettyPrinter().writeValueAsString(missionInput);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException("Failed to serialize canonical mission input", ex);
        }
    }
}
