package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos;
import com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivity;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalogEntry;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.RegisteredSkillFile;

import java.time.Duration;
import java.time.Instant;

public final class ObservabilityDtoMapper
{
    public ObservabilityDtos.SkillSummary skill(RegisteredSkillFile.Summary source)
    {
        return new ObservabilityDtos.SkillSummary(
                source.registeredName(), source.sourcePath(), "skills/" + encodePathSegment(source.registeredName()));
    }

    public ObservabilityDtos.SkillDetail skill(RegisteredSkillFile source)
    {
        return new ObservabilityDtos.SkillDetail(source.registeredName(), source.sourcePath(), source.yaml());
    }

    public ObservabilityDtos.ActiveExecution active(
            ActiveExecutionSnapshot source, Instant observedAt, LoomspanProperties.Session.Quotas quotas)
    {
        long elapsed;
        try
        {
            elapsed = Math.max(0, Duration.between(source.startedAt(), observedAt).toMillis());
        }
        catch (ArithmeticException ex)
        {
            elapsed = Long.MAX_VALUE;
        }
        return new ObservabilityDtos.ActiveExecution(
                source.sessionId(), source.traceId(), source.lastCanonicalSequence(), source.startedAt(),
                source.updatedAt(), elapsed, source.entrySkill(), "ACTIVE", source.phase(), source.summary(),
                source.activePath().stream().map(entry -> new ObservabilityDtos.FramePathEntry(
                        entry.frameId(), entry.frameType(), entry.route())).toList(),
                source.totalFrameDepth(), source.activePathTruncated(),
                new ObservabilityDtos.Usage(
                        source.usage().skillInvocations(), source.usage().toolInvocations(),
                        source.usage().linterRetries(), source.usage().modelCalls(), source.usage().providerAttempts(),
                        source.usage().promptUnits(), source.usage().completionUnits(),
                        source.usage().usageUnits(), source.usage().exactModelResponses(),
                        source.usage().heuristicModelResponses(), source.usage().unavailableModelResponses()),
                new ObservabilityDtos.QuotaLimits(
                        quotas.getMaxSkillInvocations(), quotas.getMaxToolInvocations(),
                        quotas.getMaxLinterRetries(), quotas.getMaxModelCalls(), quotas.getMaxProviderAttempts(),
                        quotas.getMaxUsageUnits()));
    }

    public ObservabilityDtos.Trace trace(FinalizedTraceCatalogEntry source)
    {
        return new ObservabilityDtos.Trace(
                source.traceId(), source.sessionId(), source.entrySkill(), source.outcome(), source.finalizedAt(), source.sizeBytes(),
                source.persistencePolicy(), source.applicationTraceExpiresAt());
    }

    public ObservabilityDtos.ActivityEnvelope activity(String instanceId, ExecutionActivity source)
    {
        return new ObservabilityDtos.ActivityEnvelope(
                instanceId,
                Long.toString(source.deliveryCursor()),
                source.sessionId(),
                source.traceId(),
                source.canonicalSequence(),
                source.timestamp(),
                source.kind(),
                source.executionStatus(),
                source.frameId(),
                source.parentFrameId(),
                source.frameType(),
                source.route(),
                source.summary(),
                java.util.Map.copyOf(source.details()));
    }

    private static String encodePathSegment(String value)
    {
        StringBuilder result = new StringBuilder();
        for (byte valueByte : value.getBytes(java.nio.charset.StandardCharsets.UTF_8))
        {
            int unsigned = valueByte & 0xff;
            if ((unsigned >= 'a' && unsigned <= 'z') || (unsigned >= 'A' && unsigned <= 'Z')
                    || (unsigned >= '0' && unsigned <= '9') || unsigned == '-' || unsigned == '_'
                    || unsigned == '.' || unsigned == '~')
            {
                result.append((char) unsigned);
            }
            else
            {
                result.append('%').append(String.format("%02X", unsigned));
            }
        }
        return result.toString();
    }
}
