package com.lokiscale.loomspan.internal.runtime.state;

import com.lokiscale.loomspan.internal.core.AdvisorTraceContext;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.DefaultExecutionTraceRecorder;
import com.lokiscale.loomspan.internal.core.ExecutionTraceRecorder;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.OperationType;
import com.lokiscale.loomspan.internal.core.TaskExecutionEvent;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.core.TraceCompletion;
import com.lokiscale.loomspan.internal.core.ToolTraceContext;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;

import java.time.Clock;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.Collection;
import java.util.LinkedHashMap;
import java.util.Set;

public class DefaultExecutionStateService implements ExecutionStateService
{
    private final Clock clock;
    private final SessionUsageService sessionUsageService;
    // Runtime observability flows through one recorder boundary so feature code does not invent parallel trace semantics.
    private final ExecutionTraceRecorder traceRecorder;

    public DefaultExecutionStateService(Clock clock)
    {
        this(clock, new NoOpSessionUsageService());
    }

    public DefaultExecutionStateService(Clock clock, SessionUsageService sessionUsageService)
    {
        this(clock, sessionUsageService, new DefaultExecutionTraceRecorder(clock));
    }

    public DefaultExecutionStateService(Clock clock,
            SessionUsageService sessionUsageService,
            ExecutionTraceRecorder traceRecorder)
    {
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
        this.traceRecorder = Objects.requireNonNull(traceRecorder, "traceRecorder must not be null");
    }

    @Override
    public ExecutionFrame openMissionFrame(LoomspanSession session, String route, Map<String, Object> parameters)
    {
        return openFrame(session, TraceFrameType.ROOT_MISSION, route, parameters);
    }

    @Override
    public ExecutionFrame openFrame(LoomspanSession session, TraceFrameType traceFrameType, String route, Map<String, Object> parameters)
    {
        Objects.requireNonNull(session, "session must not be null");
        Instant now = clock.instant();
        ExecutionFrame frame = new ExecutionFrame(
                UUID.randomUUID().toString(),
                currentFrameId(session),
                mapOperationType(traceFrameType),
                traceFrameType,
                route,
                parameters == null ? Map.of() : Map.copyOf(parameters),
                now);

        session.pushFrame(frame);
        try
        {
            traceRecorder.recordFrameOpened(session, frame);
        }
        catch (RuntimeException | Error ex)
        {
            rollbackFramePush(session, frame);
            throw ex;
        }
        return frame;
    }

    @Override
    public void closeMissionFrame(LoomspanSession session, ExecutionFrame frame)
    {
        closeFrame(session, frame, Map.of());
    }

    @Override
    public void closeFrame(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(frame, "frame must not be null");
        List<ExecutionFrame> frames = session.getFramesSnapshot();

        if (frames.isEmpty())
        {
            return;
        }
        if (frames.stream().noneMatch(frame::equals))
        {
            return;
        }

        ExecutionFrame activeFrame;
        try
        {
            activeFrame = session.peekFrame();
        }
        catch (IllegalStateException ex)
        {
            return;
        }

        if (!activeFrame.equals(frame))
        {
            throw new IllegalStateException("Attempted to close execution frame '%s' but active frame was '%s'."
                    .formatted(frame.frameId(), activeFrame.frameId()));
        }

        traceRecorder.recordFrameClosed(session, frame, metadata == null ? Map.of() : Map.copyOf(metadata));
        try
        {
            session.popFrame();
        }
        catch (IllegalStateException ex)
        {
            if (session.getFramesSnapshot().stream().noneMatch(frame::equals))
            {
                return;
            }
            throw ex;
        }
    }

    @Override
    public void storePlan(LoomspanSession session, ExecutionPlan plan)
    {
        Objects.requireNonNull(session, "session must not be null");
        session.replaceExecutionPlan(Objects.requireNonNull(plan, "plan must not be null"));
    }

    @Override
    public void clearPlan(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        session.clearExecutionPlan();
    }

    @Override
    public Optional<ExecutionPlan> currentPlan(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        return session.getExecutionPlan();
    }

    @Override
    public PlanSnapshot snapshotPlan(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        return PlanSnapshot.of(session.getExecutionPlan().orElse(null));
    }

    @Override
    public void restorePlan(LoomspanSession session, PlanSnapshot snapshot)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(snapshot, "snapshot must not be null");
        if (snapshot.plan() == null)
        {
            session.clearExecutionPlan();
            return;
        }
        session.replaceExecutionPlan(snapshot.plan());
    }

    @Override
    public SuccessfulSkillSnapshot snapshotSuccessfulSkills(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        return SuccessfulSkillSnapshot.of(session.getSuccessfulDirectSkills());
    }

    @Override
    public void restoreSuccessfulSkills(LoomspanSession session, SuccessfulSkillSnapshot snapshot)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(snapshot, "snapshot must not be null");
        if (snapshot.successfulDirectSkills() == null)
        {
            session.clearSuccessfulDirectSkills();
            return;
        }
        session.replaceSuccessfulDirectSkills(snapshot.successfulDirectSkills());
    }

    @Override
    public void logPlanCreated(LoomspanSession session, ExecutionPlan plan)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordPlanCreated(session, Objects.requireNonNull(plan, "plan must not be null"));
    }

    @Override
    public void logPlanUpdated(LoomspanSession session, ExecutionPlan plan)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordPlanUpdated(session, Objects.requireNonNull(plan, "plan must not be null"));
    }

    @Override
    public void recordPlanningEvent(LoomspanSession session,
            ExecutionFrame frame,
            TraceRecordType recordType,
            Map<String, Object> metadata,
            Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(frame, "frame must not be null");
        Objects.requireNonNull(recordType, "recordType must not be null");
        session.appendTraceRecord(recordType, frame,
                metadata == null ? Map.of() : Map.copyOf(metadata),
                payload == null ? Map.of() : payload);
    }

    @Override
    public void recordModelRequestPrepared(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordModelRequestPrepared(
                session,
                Objects.requireNonNull(frame, "frame must not be null"),
                Objects.requireNonNull(context, "context must not be null"),
                attempt,
                payload);
    }

    @Override
    public void recordModelRequestSent(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordModelRequestSent(
                session,
                Objects.requireNonNull(frame, "frame must not be null"),
                Objects.requireNonNull(context, "context must not be null"),
                attempt,
                payload);
    }

    @Override
    public void recordModelResponseReceived(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, ModelUsageRecord usage, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordModelResponseReceived(
                session,
                Objects.requireNonNull(frame, "frame must not be null"),
                Objects.requireNonNull(context, "context must not be null"),
                attempt,
                Objects.requireNonNull(usage, "usage must not be null"),
                payload);
    }

    @Override
    public void recordModelAttemptFailed(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Map<String, Object> failureMetadata, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordModelAttemptFailed(session, Objects.requireNonNull(frame, "frame must not be null"),
                Objects.requireNonNull(context, "context must not be null"), attempt,
                failureMetadata == null ? Map.of() : Map.copyOf(failureMetadata), payload);
    }

    @Override
    public void logToolCall(LoomspanSession session, TaskExecutionEvent event)
    {
        Objects.requireNonNull(session, "session must not be null");
        TaskExecutionEvent safeEvent = Objects.requireNonNull(event, "event must not be null");
        ExecutionFrame frame = requireActiveFrame(session);
        ToolTraceContext context = toolContext(safeEvent, false);
        traceRecorder.recordToolStarted(session, frame, context, safeEvent);
    }

    @Override
    public void logUnplannedToolCall(LoomspanSession session, TaskExecutionEvent event)
    {
        Objects.requireNonNull(session, "session must not be null");
        TaskExecutionEvent safeEvent = Objects.requireNonNull(event, "event must not be null");
        ExecutionFrame frame = requireActiveFrame(session);
        ToolTraceContext context = toolContext(safeEvent, true);
        traceRecorder.recordToolStarted(session, frame, context, safeEvent);
    }

    @Override
    public void logToolResult(LoomspanSession session, TaskExecutionEvent event)
    {
        Objects.requireNonNull(session, "session must not be null");
        TaskExecutionEvent safeEvent = Objects.requireNonNull(event, "event must not be null");
        traceRecorder.recordToolCompleted(
                session,
                requireActiveFrame(session),
                toolContext(safeEvent, safeEvent.linkedTaskId() == null),
                safeEvent);
    }

    @Override
    public void logToolFailure(LoomspanSession session, ToolTraceContext context, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordToolFailed(
                session,
                requireActiveFrame(session),
                Objects.requireNonNull(context, "context must not be null"),
                payload);
    }

    @Override
    public void clearSuccessfulSkills(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        session.clearSuccessfulDirectSkills();
    }

    @Override
    public Set<String> currentSuccessfulSkills(LoomspanSession session)
    {
        Objects.requireNonNull(session, "session must not be null");
        return session.getSuccessfulDirectSkills();
    }

    @Override
    public void recordSuccessfulSkill(LoomspanSession session,
            String capabilityName,
            String linkedTaskId,
            boolean unplanned)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(capabilityName, "capabilityName must not be null");
        session.addSuccessfulDirectSkill(capabilityName);
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("capabilityName", capabilityName);
        metadata.put("unplanned", unplanned);
        if (linkedTaskId != null)
        {
            metadata.put("linkedTaskId", linkedTaskId);
        }
        session.appendTraceRecord(TraceRecordType.EVIDENCE_RECORDED, Map.copyOf(metadata), Map.of(
                "successfulSkill", capabilityName,
                "successfulDirectSkills", session.getSuccessfulDirectSkills()));
    }

    @Override
    public void recordEvidenceValidation(LoomspanSession session,
            boolean passed,
            Map<String, Object> metadata,
            Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        session.appendTraceRecord(
                passed ? TraceRecordType.EVIDENCE_VALIDATION_PASSED : TraceRecordType.EVIDENCE_VALIDATION_FAILED,
                metadata == null ? Map.of() : Map.copyOf(metadata),
                payload == null ? Map.of() : payload);
    }

    @Override
    public void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome)
    {
        Objects.requireNonNull(session, "session must not be null");
        LinterOutcome recordedOutcome = Objects.requireNonNull(outcome, "outcome must not be null");
        session.setLastLinterOutcome(recordedOutcome);
        traceRecorder.recordLinterOutcome(session, recordedOutcome);
        sessionUsageService.recordLinterOutcome(session, recordedOutcome);
    }

    @Override
    public void recordOutputSchemaOutcome(LoomspanSession session, OutputSchemaOutcome outcome)
    {
        Objects.requireNonNull(session, "session must not be null");
        OutputSchemaOutcome recordedOutcome = Objects.requireNonNull(outcome, "outcome must not be null");
        session.setLastOutputSchemaOutcome(recordedOutcome);
        traceRecorder.recordOutputSchemaOutcome(session, recordedOutcome);
    }

    @Override
    public void recordAdvisorRequestMutation(LoomspanSession session, AdvisorTraceContext context, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordAdvisorRequestMutation(session, Objects.requireNonNull(context, "context must not be null"), payload);
    }

    @Override
    public void recordAdvisorResponseMutation(LoomspanSession session, AdvisorTraceContext context, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.recordAdvisorResponseMutation(session, Objects.requireNonNull(context, "context must not be null"), payload);
    }

    @Override
    public String recordFailure(LoomspanSession session, Throwable failure, Map<String, Object> payload)
    {
        return Objects.requireNonNull(session, "session must not be null").recordFailure(
                Objects.requireNonNull(failure, "failure must not be null"), payload);
    }

    @Override
    public void recordStepEvent(LoomspanSession session, ExecutionFrame frame, TraceRecordType recordType,
            Map<String, Object> metadata, Object payload)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(frame, "frame must not be null");
        Objects.requireNonNull(recordType, "recordType must not be null");
        session.appendTraceRecord(recordType, frame,
                metadata == null ? Map.of() : Map.copyOf(metadata),
                payload == null ? Map.of() : payload);
    }

    @Override
    public void finalizeTrace(LoomspanSession session, TraceCompletion completion)
    {
        Objects.requireNonNull(session, "session must not be null");
        traceRecorder.finalizeTrace(session, Objects.requireNonNull(completion, "completion must not be null"));
    }

    private String currentFrameId(LoomspanSession session)
    {
        List<ExecutionFrame> frames = session.getFramesSnapshot();
        return frames.isEmpty() ? null : frames.getFirst().frameId();
    }

    private OperationType mapOperationType(TraceFrameType traceFrameType)
    {
        if (traceFrameType == null)
        {
            return OperationType.SKILL;
        }

        return switch (traceFrameType)
        {
            case ROOT_MISSION -> OperationType.CAPABILITY;
            case SKILL_EXECUTION, PLANNING, MODEL_CALL, TOOL_INVOCATION, STEP_EXECUTION -> OperationType.SKILL;
            case RETRY -> OperationType.SUB_AGENT;
        };
    }

    private ExecutionFrame requireActiveFrame(LoomspanSession session)
    {
        return session.peekFrame();
    }

    private ToolTraceContext toolContext(TaskExecutionEvent event, boolean unplanned)
    {
        return new ToolTraceContext(event.capabilityName(), event.linkedTaskId(), unplanned);
    }

    private void rollbackFramePush(LoomspanSession session, ExecutionFrame frame)
    {
        List<ExecutionFrame> frames = session.getFramesSnapshot();
        if (frames.isEmpty() || !frame.equals(frames.getFirst()))
        {
            return;
        }
        try
        {
            session.popFrame();
        }
        catch (IllegalStateException ignored)
        {
            // Best-effort rollback only; preserve the original recorder failure.
        }
    }
}
