package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;
import org.springframework.lang.Nullable;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Call-local model trace state. One instance belongs to one logical retry
 * sequence and is copied through Spring AI's advisor context.
 */
public final class ModelTraceContext
{
    public static final String REQUEST_CONTEXT_KEY = "loomspan.model-trace.context";
    public static final String RESPONSE_ATTEMPT_CONTEXT_KEY = "loomspan.model-trace.attempt";

    private final ModelExecutionIdentity identity;
    private final String skillName;
    private final String segment;
    private final String retrySequenceId;
    private final AtomicInteger attemptCounter = new AtomicInteger();

    public ModelTraceContext(ModelExecutionIdentity identity, String skillName, String segment)
    {
        this(identity, skillName, segment, UUID.randomUUID().toString());
    }

    ModelTraceContext(ModelExecutionIdentity identity, String skillName, String segment, String retrySequenceId)
    {
        this.identity = Objects.requireNonNull(identity, "identity must not be null");
        this.skillName = requireNonBlank(skillName, "skillName");
        this.segment = requireNonBlank(segment, "segment");
        this.retrySequenceId = requireNonBlank(retrySequenceId, "retrySequenceId");
    }

    public ModelExecutionIdentity identity()
    {
        return identity;
    }

    public String skillName()
    {
        return skillName;
    }

    public String segment()
    {
        return segment;
    }

    public String retrySequenceId()
    {
        return retrySequenceId;
    }

    public Map<String, Object> nextAttempt()
    {
        return nextAttempt(1);
    }

    public Map<String, Object> nextAttempt(int providerAttemptNumber)
    {
        int attemptNumber = attemptCounter.incrementAndGet();
        String reason = attemptNumber == 1 ? "INITIAL"
                : providerAttemptNumber > 1 ? "PROVIDER_RETRY" : "SEMANTIC_RETRY";
        return attempt(UUID.randomUUID().toString(), attemptNumber, reason, providerAttemptNumber);
    }

    public Map<String, Object> metadata(Map<String, Object> attempt)
    {
        Map<String, Object> metadata = new LinkedHashMap<>(identity.metadata());
        metadata.put("skillName", skillName);
        metadata.put("segment", segment);
        metadata.putAll(requireAttempt(attempt));
        return Map.copyOf(metadata);
    }

    public Map<String, Object> responseMetadata(Map<String, Object> attempt, ModelUsageRecord usage)
    {
        Map<String, Object> metadata = new LinkedHashMap<>(metadata(attempt));
        ModelUsageRecord safeUsage = Objects.requireNonNull(usage, "usage must not be null");
        metadata.put("usage", Map.of(
                "promptUnits", safeUsage.promptUnits(),
                "completionUnits", safeUsage.completionUnits(),
                "totalUnits", safeUsage.totalUnits(),
                "precision", safeUsage.precision().name()));
        return Map.copyOf(metadata);
    }

    public static Map<String, Object> attemptFrom(@Nullable Map<String, Object> responseContext)
    {
        if (responseContext == null)
        {
            return Map.of();
        }
        Object value = responseContext.get(RESPONSE_ATTEMPT_CONTEXT_KEY);
        if (!(value instanceof Map<?, ?> raw))
        {
            return Map.of();
        }
        LinkedHashMap<String, Object> attempt = new LinkedHashMap<>();
        raw.forEach((key, item) ->
        {
            if (key instanceof String stringKey)
            {
                attempt.put(stringKey, item);
            }
        });
        return requireAttempt(attempt);
    }

    private Map<String, Object> attempt(String attemptId, int attemptNumber, String reason, int providerAttemptNumber)
    {
        return Map.of(
                "retrySequenceId", retrySequenceId,
                "attemptId", requireNonBlank(attemptId, "attemptId"),
                "attemptNumber", attemptNumber,
                "attemptReason", reason,
                "providerAttemptNumber", providerAttemptNumber);
    }

    private static Map<String, Object> requireAttempt(Map<String, Object> attempt)
    {
        if (attempt == null || attempt.isEmpty())
        {
            return Map.of();
        }
        String retrySequenceId = requireNonBlank((String) attempt.get("retrySequenceId"), "retrySequenceId");
        String attemptId = requireNonBlank((String) attempt.get("attemptId"), "attemptId");
        Object rawAttemptNumber = attempt.get("attemptNumber");
        if (!(rawAttemptNumber instanceof Number number) || number.intValue() <= 0)
        {
            throw new IllegalArgumentException("attemptNumber must be greater than zero");
        }
        String reason = requireNonBlank((String) attempt.get("attemptReason"), "attemptReason");
        Object rawProviderAttemptNumber = attempt.get("providerAttemptNumber");
        if (!(rawProviderAttemptNumber instanceof Number providerNumber) || providerNumber.intValue() <= 0)
        {
            throw new IllegalArgumentException("providerAttemptNumber must be greater than zero");
        }
        return Map.of(
                "retrySequenceId", retrySequenceId,
                "attemptId", attemptId,
                "attemptNumber", number.intValue(),
                "attemptReason", reason,
                "providerAttemptNumber", providerNumber.intValue());
    }

    private static String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }
}
