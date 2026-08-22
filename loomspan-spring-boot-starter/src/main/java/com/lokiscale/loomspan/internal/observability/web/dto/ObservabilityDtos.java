package com.lokiscale.loomspan.internal.observability.web.dto;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivityKind;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Map;

public final class ObservabilityDtos
{
    private ObservabilityDtos() {}

    public record InstanceStatus(
            String instanceId,
            String consoleCompatibilityVersion,
            Instant observedAt,
            boolean liveMonitoringAvailable,
            int registeredSkillCount,
            int activeExecutionCount,
            int catalogedTraceCount,
            TracePersistencePolicy tracePersistencePolicy,
            Duration completionGraceTtl,
            Duration traceCatalogMetadataTtl) {}

    public record SkillSummary(String registeredName, String sourcePath, String href) {}
    public record SkillDetail(String registeredName, String sourcePath, String yaml) {}
    public record FramePathEntry(String frameId, TraceFrameType frameType, String route) {}
    public record QuotaLimits(
            int maxSkillInvocations,
            int maxToolInvocations,
            int maxLinterRetries,
            int maxModelCalls,
            int maxProviderAttempts,
            int maxUsageUnits) {}
    public record Usage(
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
            int unavailableModelResponses) {}
    public record ActiveExecution(
            String sessionId,
            String traceId,
            long lastCanonicalSequence,
            Instant startedAt,
            Instant updatedAt,
            long elapsedMillis,
            String entrySkill,
            String status,
            String phase,
            String summary,
            List<FramePathEntry> activePath,
            int totalFrameDepth,
            boolean activePathTruncated,
            Usage usage,
            QuotaLimits configuredLimits) {}
    public record Trace(
            String traceId,
            String sessionId,
            String entrySkill,
            TraceOutcome outcome,
            Instant finalizedAt,
            long sizeBytes,
            TracePersistencePolicy persistencePolicy,
            Instant applicationTraceExpiresAt) {}
    public record Page<T>(
            List<T> items,
            boolean hasMore,
            @JsonInclude(JsonInclude.Include.ALWAYS) String nextCursor,
            Instant observedAt) {}
    public record ActivePage(
            List<ActiveExecution> items,
            boolean hasMore,
            @JsonInclude(JsonInclude.Include.ALWAYS) String nextCursor,
            Instant observedAt,
            @JsonInclude(JsonInclude.Include.NON_NULL) String resumeCursor) {}

    public record ActivityHandshake(
            String instanceId,
            Instant observedAt,
            String afterCursor) {}

    @JsonInclude(JsonInclude.Include.NON_NULL)
    public record ActivityEnvelope(
            String instanceId,
            String cursor,
            String sessionId,
            String traceId,
            Long canonicalSequence,
            Instant timestamp,
            ExecutionActivityKind kind,
            String executionStatus,
            String frameId,
            String parentFrameId,
            TraceFrameType frameType,
            String route,
            String summary,
            Map<String, Object> details) {}
}
