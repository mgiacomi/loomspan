package com.lokiscale.loomspan.internal.runtime.usage;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

public record SessionUsageSnapshot(
        int skillInvocations,
        int toolInvocations,
        int linterRetries,
        int modelCalls,
        int providerAttempts,
        int promptUnits,
        int completionUnits,
        int usageUnits,
        int exactModelResponses,
        int heuristicModelResponses,
        int unavailableModelResponses)
{
    public SessionUsageSnapshot
    {
        validateNonNegative(skillInvocations, "skillInvocations");
        validateNonNegative(toolInvocations, "toolInvocations");
        validateNonNegative(linterRetries, "linterRetries");
        validateNonNegative(modelCalls, "modelCalls");
        validateNonNegative(providerAttempts, "providerAttempts");
        validateNonNegative(promptUnits, "promptUnits");
        validateNonNegative(completionUnits, "completionUnits");
        validateNonNegative(usageUnits, "usageUnits");
        validateNonNegative(exactModelResponses, "exactModelResponses");
        validateNonNegative(heuristicModelResponses, "heuristicModelResponses");
        validateNonNegative(unavailableModelResponses, "unavailableModelResponses");
    }

    public static SessionUsageSnapshot empty()
    {
        return new SessionUsageSnapshot(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0);
    }

    public SessionUsageSnapshot incrementSkillInvocations()
    {
        return new SessionUsageSnapshot(
                skillInvocations + 1,
                toolInvocations,
                linterRetries,
                modelCalls,
                providerAttempts,
                promptUnits,
                completionUnits,
                usageUnits,
                exactModelResponses,
                heuristicModelResponses,
                unavailableModelResponses);
    }

    public SessionUsageSnapshot incrementToolInvocations()
    {
        return new SessionUsageSnapshot(
                skillInvocations,
                toolInvocations + 1,
                linterRetries,
                modelCalls,
                providerAttempts,
                promptUnits,
                completionUnits,
                usageUnits,
                exactModelResponses,
                heuristicModelResponses,
                unavailableModelResponses);
    }

    public SessionUsageSnapshot incrementLinterRetries()
    {
        return new SessionUsageSnapshot(
                skillInvocations,
                toolInvocations,
                linterRetries + 1,
                modelCalls,
                providerAttempts,
                promptUnits,
                completionUnits,
                usageUnits,
                exactModelResponses,
                heuristicModelResponses,
                unavailableModelResponses);
    }

    public SessionUsageSnapshot recordModelUsage(ModelUsageRecord usageRecord)
    {
        Objects.requireNonNull(usageRecord, "usageRecord must not be null");
        return new SessionUsageSnapshot(
                skillInvocations,
                toolInvocations,
                linterRetries,
                modelCalls + 1,
                providerAttempts,
                promptUnits + usageRecord.promptUnits(),
                completionUnits + usageRecord.completionUnits(),
                usageUnits + usageRecord.totalUnits(),
                exactModelResponses + (usageRecord.precision() == UsagePrecision.EXACT ? 1 : 0),
                heuristicModelResponses + (usageRecord.precision() == UsagePrecision.HEURISTIC ? 1 : 0),
                unavailableModelResponses + (usageRecord.precision() == UsagePrecision.UNAVAILABLE ? 1 : 0));
    }

    public SessionUsageSnapshot incrementProviderAttempts()
    {
        return new SessionUsageSnapshot(skillInvocations, toolInvocations, linterRetries, modelCalls,
                providerAttempts + 1, promptUnits, completionUnits, usageUnits, exactModelResponses,
                heuristicModelResponses, unavailableModelResponses);
    }

    /**
     * Returns an immutable map representation using the trace contract's field
     * names. The session usage snapshot stores the model usage total under
     * {@code usageUnits}, but the trace contract (and the Go analysis processor)
     * expects {@code totalUnits} in the {@code sessionUsageSnapshot} metadata of
     * a {@code TRACE_COMPLETED} record. This method performs the rename so the
     * serialized form matches what consumers downstream of the NDJSON expect.
     *
     * @return an immutable, ordered map suitable for inclusion in trace metadata
     */
    public Map<String, Object> toTraceMap()
    {
        LinkedHashMap<String, Object> map = new LinkedHashMap<>();
        map.put("skillInvocations", skillInvocations);
        map.put("toolInvocations", toolInvocations);
        map.put("linterRetries", linterRetries);
        map.put("modelCalls", modelCalls);
        map.put("providerAttempts", providerAttempts);
        map.put("promptUnits", promptUnits);
        map.put("completionUnits", completionUnits);
        map.put("totalUnits", usageUnits);
        map.put("exactModelResponses", exactModelResponses);
        map.put("heuristicModelResponses", heuristicModelResponses);
        map.put("unavailableModelResponses", unavailableModelResponses);
        return Collections.unmodifiableMap(map);
    }

    private static void validateNonNegative(int value, String name)
    {
        if (value < 0)
        {
            throw new IllegalArgumentException(name + " must not be negative");
        }
    }
}
