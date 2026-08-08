package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.springframework.lang.Nullable;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

final class ExecutionProjectionState
{
    final String sessionId;
    final LinkedHashMap<String, ActiveExecutionSnapshot.FramePathEntry> frames = new LinkedHashMap<>();
    String traceId;
    Instant startedAt;
    final String entrySkill;
    String phase = "STARTING";
    String summary = "Execution started";
    SessionUsageSnapshot usage = SessionUsageSnapshot.empty();
    TraceOutcome outcome;

    ExecutionProjectionState(String sessionId, String entrySkill)
    {
        this.sessionId = sessionId;
        if (entrySkill == null || entrySkill.isBlank())
        {
            throw new IllegalArgumentException("entrySkill must not be blank");
        }
        this.entrySkill = entrySkill;
    }

    void openFrame(String frameId, TraceFrameType frameType, String route)
    {
        if (frameId != null && frameType != null && route != null)
        {
            frames.put(frameId, new ActiveExecutionSnapshot.FramePathEntry(frameId, frameType, route));
        }
    }

    void closeFrame(@Nullable String frameId)
    {
        if (frameId != null)
        {
            frames.remove(frameId);
        }
    }

    Map<String, ActiveExecutionSnapshot.FramePathEntry> frameView()
    {
        return Map.copyOf(frames);
    }
}
