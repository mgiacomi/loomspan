package com.lokiscale.loomspan.internal.core;

import java.util.Map;

import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;

public interface ExecutionTraceRecorder
{
    void recordFrameOpened(LoomspanSession session, ExecutionFrame frame);

    void recordFrameClosed(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata);

    void recordModelRequestPrepared(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload);

    void recordModelRequestSent(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Object payload);

    void recordModelResponseReceived(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, ModelUsageRecord usage, Object payload);

    void recordModelAttemptFailed(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
            Map<String, Object> attempt, Map<String, Object> failureMetadata, Object payload);

    void recordPlanCreated(LoomspanSession session, ExecutionPlan plan, Map<String, Object> acceptedAttempt);

    void recordPlanUpdated(LoomspanSession session, ExecutionPlan plan);

    void recordToolStarted(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload);

    void recordToolCompleted(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload);

    void recordToolFailed(LoomspanSession session, ExecutionFrame frame, ToolTraceContext context, Object payload);

    void recordAdvisorRequestMutation(LoomspanSession session, AdvisorTraceContext context, Object payload);

    void recordAdvisorResponseMutation(LoomspanSession session, AdvisorTraceContext context, Object payload);

    void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome);

    void recordOutputSchemaOutcome(LoomspanSession session, OutputSchemaOutcome outcome);

    void finalizeTrace(LoomspanSession session, TraceCompletion completion);
}
