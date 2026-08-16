package com.lokiscale.loomspan.internal.runtime.state;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.AdvisorTraceContext;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.TaskExecutionEvent;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceCompletion;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.core.ToolTraceContext;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;
import org.springframework.lang.Nullable;

import java.util.Map;
import java.util.Optional;
import java.util.Set;

public interface ExecutionStateService
{
    ExecutionFrame openMissionFrame(LoomspanSession session, String route, Map<String, Object> parameters);

    ExecutionFrame openFrame(LoomspanSession session, TraceFrameType traceFrameType, String route, Map<String, Object> parameters);

    void closeMissionFrame(LoomspanSession session, ExecutionFrame frame);

    void closeFrame(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata);

    void storePlan(LoomspanSession session, ExecutionPlan plan);

    void clearPlan(LoomspanSession session);

    Optional<ExecutionPlan> currentPlan(LoomspanSession session);

    PlanSnapshot snapshotPlan(LoomspanSession session);

    void restorePlan(LoomspanSession session, PlanSnapshot snapshot);

    SuccessfulSkillSnapshot snapshotSuccessfulSkills(LoomspanSession session);

    void restoreSuccessfulSkills(LoomspanSession session, SuccessfulSkillSnapshot snapshot);

    void logPlanCreated(LoomspanSession session, ExecutionPlan plan, Map<String, Object> acceptedAttempt);

    void logPlanUpdated(LoomspanSession session, ExecutionPlan plan);

    void recordPlanningEvent(LoomspanSession session,
            ExecutionFrame frame,
            TraceRecordType recordType,
            Map<String, Object> metadata,
            Object payload);

    void recordModelRequestPrepared(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload);

    void recordModelRequestSent(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload);

    void recordModelResponseReceived(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, ModelUsageRecord usage, Object payload);

    void recordModelAttemptFailed(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Map<String, Object> failureMetadata, Object payload);

    void logToolCall(LoomspanSession session, TaskExecutionEvent event);

    void logUnplannedToolCall(LoomspanSession session, TaskExecutionEvent event);

    void logToolResult(LoomspanSession session, TaskExecutionEvent event);

    void logToolFailure(LoomspanSession session, ToolTraceContext context, Object payload);

    void clearSuccessfulSkills(LoomspanSession session);

    Set<String> currentSuccessfulSkills(LoomspanSession session);

    void recordSuccessfulSkill(LoomspanSession session,
            String capabilityName,
            @Nullable String linkedTaskId,
            boolean unplanned);

    void recordEvidenceValidation(LoomspanSession session,
            boolean passed,
            Map<String, Object> metadata,
            Object payload);

    void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome);

    void recordOutputSchemaOutcome(LoomspanSession session, OutputSchemaOutcome outcome);

    void recordAdvisorRequestMutation(LoomspanSession session, AdvisorTraceContext context, Object payload);

    void recordAdvisorResponseMutation(LoomspanSession session, AdvisorTraceContext context, Object payload);

    String recordFailure(LoomspanSession session, Throwable failure, Map<String, Object> payload);

    void recordStepEvent(LoomspanSession session, ExecutionFrame frame, TraceRecordType recordType,
            Map<String, Object> metadata, Object payload);

    void finalizeTrace(LoomspanSession session, TraceCompletion completion);
}
