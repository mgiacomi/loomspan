package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;

import java.time.Clock;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

public final class DefaultExecutionTraceRecorder implements ExecutionTraceRecorder
{
    private final Clock clock;

    public DefaultExecutionTraceRecorder(Clock clock)
    {
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
    }

    @Override
    public void recordFrameOpened(LoomspanSession session, ExecutionFrame frame)
    {
        recordAgainstFrame(session, frame, TraceRecordType.FRAME_OPENED, Map.of(
                "openedAt", frame.openedAt().toString(),
                "operationType", frame.operationType().name(),
                "frameType", frame.traceFrameType().name()), frame.parameters());
    }

    @Override
    public void recordFrameClosed(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata)
    {
        recordAgainstFrame(session, frame, TraceRecordType.FRAME_CLOSED, metadata, Map.of(
                "frameId", frame.frameId(),
                "route", frame.route(),
                "closedAt", clock.instant().toString()));
    }

    @Override
    public void recordModelRequestSent(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload)
    {
        recordAgainstFrame(session, frame, TraceRecordType.MODEL_REQUEST_SENT, context.metadata(attempt), payload);
    }

    @Override
    public void recordModelResponseReceived(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, ModelUsageRecord usage, Object payload)
    {
        recordAgainstFrame(session, frame, TraceRecordType.MODEL_RESPONSE_RECEIVED, context.responseMetadata(attempt, usage), payload);
    }

    @Override
    public void recordModelAttemptFailed(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Map<String, Object> failureMetadata, Object payload)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>(context.metadata(attempt));
        metadata.putAll(failureMetadata);
        recordAgainstFrame(session, frame, TraceRecordType.MODEL_ATTEMPT_FAILED, Map.copyOf(metadata), payload);
    }

    @Override
    public void recordPlanCreated(LoomspanSession session, ExecutionPlan plan, Map<String, Object> acceptedAttempt)
    {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("planId", requireNonBlank(plan.planId(), "planId"));
        Map<String, Object> safeAttempt = Objects.requireNonNull(acceptedAttempt, "acceptedAttempt must not be null");
        metadata.put("attemptId", requireNonBlank((String) safeAttempt.get("attemptId"), "attemptId"));
        metadata.put("retrySequenceId", requireNonBlank((String) safeAttempt.get("retrySequenceId"), "retrySequenceId"));
        recordOnPlanFrame(session, TraceRecordType.PLAN_CREATED, metadata, plan);
    }

    @Override
    public void recordPlanUpdated(LoomspanSession session, ExecutionPlan plan)
    {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("planId", plan.planId());
        recordOnPlanFrame(session, TraceRecordType.PLAN_UPDATED, metadata, plan);
    }

    @Override
    public void recordToolStarted(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload)
    {
        recordAgainstFrame(session, frame, TraceRecordType.TOOL_CALL_STARTED, context.metadata(), payload);
    }

    @Override
    public void recordToolCompleted(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload)
    {
        recordAgainstFrame(session, frame, TraceRecordType.TOOL_CALL_COMPLETED, context.metadata(), payload);
    }

    @Override
    public void recordToolFailed(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload)
    {
        recordAgainstFrame(session, frame, TraceRecordType.TOOL_CALL_FAILED, context.metadata(), payload);
    }

    @Override
    public void recordAdvisorRequestMutation(LoomspanSession session, AdvisorTraceContext context, Object payload)
    {
        recordOnActiveFrame(session, TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED, context.metadata(), payload);
    }

    @Override
    public void recordAdvisorResponseMutation(LoomspanSession session, AdvisorTraceContext context, Object payload)
    {
        recordOnActiveFrame(session, TraceRecordType.ADVISOR_RESPONSE_MUTATION_RECORDED, context.metadata(), payload);
    }

    @Override
    public void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome)
    {
        if (outcome.status() == LinterOutcomeStatus.EXHAUSTED)
        {
            session.markTraceErrored();
        }

        recordOnActiveFrame(session, TraceRecordType.LINTER_RECORDED, Map.of(
                "skillName", outcome.skillName(),
                "status", outcome.status().name()), outcome);
    }

    @Override
    public void recordOutputSchemaOutcome(LoomspanSession session, OutputSchemaOutcome outcome)
    {
        if (outcome.status() == OutputSchemaOutcomeStatus.EXHAUSTED)
        {
            session.markTraceErrored();
        }

        recordOnActiveFrame(session, TraceRecordType.STRUCTURED_OUTPUT_RECORDED, Map.of(
                "skillName", outcome.skillName(),
                "status", outcome.status().name()), outcome);
    }

    @Override
    public void finalizeTrace(LoomspanSession session, TraceCompletion completion)
    {
        session.finalizeTrace(Objects.requireNonNull(completion, "completion must not be null"));
    }

    private void recordAgainstFrame(LoomspanSession session,
            ExecutionFrame frame,
            TraceRecordType type,
            Map<String, Object> metadata,
            Object payload)
    {
        session.appendTraceRecord(type, Objects.requireNonNull(frame, "frame must not be null"), augmentMetadata(metadata), payload);
    }

    private void recordOnActiveFrame(LoomspanSession session,
            TraceRecordType type,
            Map<String, Object> metadata,
            Object payload)
    {
        session.appendTraceRecord(type, augmentMetadata(metadata), payload);
    }

    private void recordOnPlanFrame(LoomspanSession session,
            TraceRecordType type,
            Map<String, Object> metadata,
            Object payload)
    {
        ExecutionFrame frame = session.getFramesSnapshot().stream()
                .filter(candidate -> candidate.traceFrameType() == TraceFrameType.PLANNING)
                .findFirst()
                .orElseGet(() -> session.getFramesSnapshot().stream()
                        .filter(candidate -> candidate.traceFrameType() == TraceFrameType.ROOT_MISSION)
                        .findFirst()
                        .orElseGet(() -> session.getFramesSnapshot().stream()
                                .filter(candidate -> candidate.traceFrameType() != TraceFrameType.MODEL_CALL)
                                .findFirst()
                                .orElse(null)));

        if (frame == null)
        {
            recordOnActiveFrame(session, type, metadata, payload);
            return;
        }

        recordAgainstFrame(session, frame, type, metadata, payload);
    }

    private Map<String, Object> augmentMetadata(Map<String, Object> metadata)
    {
        Map<String, Object> safeMetadata = new LinkedHashMap<>();
        if (metadata != null)
        {
            safeMetadata.putAll(metadata);
        }
        safeMetadata.putIfAbsent("recordedAt", clock.instant().toString());
        safeMetadata.forEach((key, value) ->
        {
            Objects.requireNonNull(key, "metadata key must not be null");
            Objects.requireNonNull(value, "metadata value must not be null");
        });
        return Collections.unmodifiableMap(safeMetadata);
    }

    private String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }
}
