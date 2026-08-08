package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.springframework.lang.Nullable;

import java.time.Instant;
import java.util.List;
import java.util.Objects;

public record ActiveExecutionSnapshot(
        String sessionId,
        String traceId,
        long registryOrdinal,
        long lastCanonicalSequence,
        Instant startedAt,
        Instant updatedAt,
        String entrySkill,
        String phase,
        String summary,
        List<FramePathEntry> activePath,
        int totalFrameDepth,
        boolean activePathTruncated,
        SessionUsageSnapshot usage,
        @Nullable TraceOutcome outcome)
{
    public ActiveExecutionSnapshot
    {
        sessionId = requireNonBlank(sessionId, "sessionId");
        traceId = requireNonBlank(traceId, "traceId");
        if (registryOrdinal < 0)
        {
            throw new IllegalArgumentException("registryOrdinal must not be negative");
        }
        if (lastCanonicalSequence <= 0)
        {
            throw new IllegalArgumentException("lastCanonicalSequence must be positive");
        }
        Objects.requireNonNull(startedAt, "startedAt must not be null");
        Objects.requireNonNull(updatedAt, "updatedAt must not be null");
        entrySkill = requireNonBlank(entrySkill, "entrySkill");
        phase = ExecutionObservationLimits.truncate(requireNonBlank(phase, "phase"),
                ExecutionObservationLimits.TEXT_CODE_POINTS);
        summary = ExecutionObservationLimits.truncate(summary, ExecutionObservationLimits.SUMMARY_CODE_POINTS);
        activePath = activePath == null ? List.of() : List.copyOf(activePath);
        if (activePath.size() > ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES)
        {
            throw new IllegalArgumentException("activePath exceeds the retained path limit");
        }
        if (totalFrameDepth < activePath.size())
        {
            throw new IllegalArgumentException("totalFrameDepth must cover activePath");
        }
        usage = usage == null ? SessionUsageSnapshot.empty() : usage;
    }

    ActiveExecutionSnapshot withRegistryOrdinal(long ordinal)
    {
        return new ActiveExecutionSnapshot(
                sessionId, traceId, ordinal, lastCanonicalSequence, startedAt, updatedAt, entrySkill,
                phase, summary, activePath, totalFrameDepth, activePathTruncated, usage, outcome);
    }

    public record FramePathEntry(String frameId, TraceFrameType frameType, String route)
    {
        public FramePathEntry
        {
            frameId = requireNonBlank(frameId, "frameId");
            Objects.requireNonNull(frameType, "frameType must not be null");
            route = ExecutionObservationLimits.truncate(requireNonBlank(route, "route"),
                    ExecutionObservationLimits.TEXT_CODE_POINTS);
        }
    }

    private static String requireNonBlank(String value, String name)
    {
        Objects.requireNonNull(value, name + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(name + " must not be blank");
        }
        return value;
    }

}
