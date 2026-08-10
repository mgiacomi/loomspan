package com.lokiscale.loomspan.internal.runtime.trace;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;

import java.util.LinkedHashMap;
import java.util.Collections;
import java.util.Map;
import java.util.Objects;

/**
 * Immutable, run-start snapshot of the configured execution quotas recorded in
 * current-run diagnostic traces. This is an internal trace-production value,
 * not an application API or supported SPI.
 */
public record ConfiguredLimitsSnapshot(
        int maxSkillInvocations,
        int maxToolInvocations,
        int maxLinterRetries,
        int maxModelCalls,
        int maxProviderAttempts,
        int maxUsageUnits)
{
    public ConfiguredLimitsSnapshot
    {
        requireNonNegative(maxSkillInvocations, "maxSkillInvocations");
        requireNonNegative(maxToolInvocations, "maxToolInvocations");
        requireNonNegative(maxLinterRetries, "maxLinterRetries");
        requireNonNegative(maxModelCalls, "maxModelCalls");
        requireNonNegative(maxProviderAttempts, "maxProviderAttempts");
        requireNonNegative(maxUsageUnits, "maxUsageUnits");
    }

    public static ConfiguredLimitsSnapshot from(LoomspanProperties.Session.Quotas quotas)
    {
        Objects.requireNonNull(quotas, "quotas must not be null");
        return new ConfiguredLimitsSnapshot(
                quotas.getMaxSkillInvocations(),
                quotas.getMaxToolInvocations(),
                quotas.getMaxLinterRetries(),
                quotas.getMaxModelCalls(),
                quotas.getMaxProviderAttempts(),
                quotas.getMaxUsageUnits());
    }

    Map<String, Integer> asMetadata()
    {
        LinkedHashMap<String, Integer> values = new LinkedHashMap<>();
        values.put("maxSkillInvocations", maxSkillInvocations);
        values.put("maxToolInvocations", maxToolInvocations);
        values.put("maxLinterRetries", maxLinterRetries);
        values.put("maxModelCalls", maxModelCalls);
        values.put("maxProviderAttempts", maxProviderAttempts);
        values.put("maxUsageUnits", maxUsageUnits);
        return Collections.unmodifiableMap(values);
    }

    private static void requireNonNegative(int value, String name)
    {
        if (value < 0)
        {
            throw new IllegalArgumentException(name + " must be zero or greater");
        }
    }
}
