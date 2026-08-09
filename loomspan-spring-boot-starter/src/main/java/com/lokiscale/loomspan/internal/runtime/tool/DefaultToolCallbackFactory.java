package com.lokiscale.loomspan.internal.runtime.tool;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.TaskExecutionEvent;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.ToolTraceContext;
import com.lokiscale.loomspan.internal.core.TraceFailureMetadata;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpUsageMetricsRecorder;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.UsageMetricsRecorder;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.springframework.ai.chat.model.ToolContext;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.function.FunctionToolCallback;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

public class DefaultToolCallbackFactory implements ToolCallbackFactory
{
    public static final String STEP_LOOP_TASK_ID_CONTEXT_KEY = "loomspan.stepLoop.taskId";

    private final CapabilityExecutionRouter capabilityExecutionRouter;
    private final PlanningService planningService;
    private final ExecutionStateService executionStateService;
    private final SessionUsageService sessionUsageService;
    private final UsageMetricsRecorder usageMetricsRecorder;

    public DefaultToolCallbackFactory(CapabilityExecutionRouter capabilityExecutionRouter,
            PlanningService planningService,
            ExecutionStateService executionStateService)
    {
        this(capabilityExecutionRouter, planningService, executionStateService, new NoOpSessionUsageService(), new NoOpUsageMetricsRecorder());
    }

    public DefaultToolCallbackFactory(CapabilityExecutionRouter capabilityExecutionRouter,
            PlanningService planningService,
            ExecutionStateService executionStateService,
            SessionUsageService sessionUsageService,
            UsageMetricsRecorder usageMetricsRecorder)
    {
        this.capabilityExecutionRouter = Objects.requireNonNull(capabilityExecutionRouter, "capabilityExecutionRouter must not be null");
        this.planningService = Objects.requireNonNull(planningService, "planningService must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
        this.usageMetricsRecorder = Objects.requireNonNull(usageMetricsRecorder, "usageMetricsRecorder must not be null");
    }

    @Override
    public List<ToolCallback> createToolCallbacks(LoomspanSession session,
            YamlSkillDefinition definition,
            List<CapabilityMetadata> capabilities,
            @Nullable Authentication authentication)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(definition, "definition must not be null");
        Objects.requireNonNull(capabilities, "capabilities must not be null");

        return capabilities.stream()
                .map(capability -> toToolCallback(capability, session, definition, authentication))
                .toList();
    }

    private ToolCallback toToolCallback(CapabilityMetadata capability,
            LoomspanSession session,
            YamlSkillDefinition definition,
            @Nullable Authentication authentication)
    {
        ToolCallback callback = FunctionToolCallback.<Map<String, Object>, Object>builder(
                capability.tool().name(),
                (arguments, toolContext) -> invokeCapability(capability, arguments, session, definition, authentication, toolContext))
                .description(capability.tool().description())
                .inputType(new ParameterizedTypeReference<Map<String, Object>>()
                {
                })
                .inputSchema(capability.tool().inputSchema())
                .build();

        return new ContractAwareToolCallback(callback, capability.inputContract());
    }

    private Object invokeCapability(CapabilityMetadata capability,
            Map<String, Object> arguments,
            LoomspanSession session,
            YamlSkillDefinition definition,
            @Nullable Authentication authentication,
            @Nullable ToolContext toolContext)
    {
        Map<String, Object> safeArguments = arguments == null ? Map.of() : arguments;
        String currentSkillName = currentSkillName(session);
        String boundTaskId = stepLoopTaskId(toolContext);
        sessionUsageService.recordToolCall(session, currentSkillName, capability.name());
        String linkedTaskId = boundTaskId;

        if (linkedTaskId == null)
        {
            var startedPlan = planningService.markToolStarted(session, capability, safeArguments);
            linkedTaskId = startedPlan.flatMap(plan -> plan.activeTask().map(task -> task.taskId())).orElse(null);
        }

        ExecutionFrame toolFrame = executionStateService.openFrame(
                session,
                TraceFrameType.TOOL_INVOCATION,
                capability.name(),
                toolFrameParameters(safeArguments, linkedTaskId));

        String toolFrameStatus = "completed";
        Throwable toolFailure = null;

        try
        {
            if (linkedTaskId == null)
            {
                executionStateService.logUnplannedToolCall(session, TaskExecutionEvent.unlinked(
                        capability.name(),
                        Map.of("arguments", safeArguments),
                        "No unique ready task matched this tool call"));
            }
            else
            {
                executionStateService.logToolCall(session, TaskExecutionEvent.linked(
                        capability.name(),
                        linkedTaskId,
                        Map.of("arguments", safeArguments),
                        null));
            }

            Object result = capabilityExecutionRouter.execute(capability, safeArguments, session, authentication);
            if (linkedTaskId != null && boundTaskId == null)
            {
                planningService.markToolCompleted(session, linkedTaskId, capability.name(), result);
            }
            else if (linkedTaskId == null)
            {
                executionStateService.recordSuccessfulSkill(session, capability.name(), null, true);
            }

            usageMetricsRecorder.recordToolInvocation(currentSkillName, capability.name(), "success");
            executionStateService.logToolResult(session, linkedTaskId == null
                    ? TaskExecutionEvent.unlinked(capability.name(), Map.of("result", result), null)
                    : TaskExecutionEvent.linked(capability.name(), linkedTaskId, Map.of("result", result), null));

            return result;
        }
        catch (RuntimeException | Error ex)
        {
            toolFailure = ex;
            toolFrameStatus = Thread.currentThread().isInterrupted() ? "aborted" : "failed";

            if (linkedTaskId != null && boundTaskId == null && ex instanceof RuntimeException runtimeException)
            {
                planningService.markToolFailed(session, linkedTaskId, capability.name(), runtimeException);
            }

            usageMetricsRecorder.recordToolInvocation(currentSkillName, capability.name(), "failure");
            LinkedHashMap<String, Object> failureMetadata = new LinkedHashMap<>();
            failureMetadata.put("capabilityName", capability.name());

            if (linkedTaskId != null)
            {
                failureMetadata.put("linkedTaskId", linkedTaskId);
            }
            TraceFailureMetadata.addTo(failureMetadata, ex, "Tool execution failed");

            executionStateService.logToolFailure(
                    session,
                    new ToolTraceContext(capability.name(), linkedTaskId, linkedTaskId == null),
                    Map.of("arguments", safeArguments, "failure", failureMetadata));

            LinkedHashMap<String, Object> errorPayload = new LinkedHashMap<>();
            errorPayload.put("tool", capability.name());

            if (linkedTaskId != null)
            {
                errorPayload.put("linkedTaskId", linkedTaskId);
            }
            TraceFailureMetadata.addTo(errorPayload, ex, "Tool execution failed");
            executionStateService.recordFailure(session, ex, errorPayload);
            throw ex;
        }
        finally
        {
            executionStateService.closeFrame(session, toolFrame, closeMetadata(toolFrameStatus, toolFailure));
        }
    }

    @Nullable
    private String stepLoopTaskId(@Nullable ToolContext toolContext)
    {
        if (toolContext == null)
        {
            return null;
        }

        Object taskId = toolContext.getContext().get(STEP_LOOP_TASK_ID_CONTEXT_KEY);
        return taskId instanceof String value && !value.isBlank() ? value : null;
    }

    private Map<String, Object> toolFrameParameters(Map<String, Object> arguments, @Nullable String linkedTaskId)
    {
        LinkedHashMap<String, Object> parameters = new LinkedHashMap<>();
        parameters.put("arguments", arguments);

        if (linkedTaskId != null)
        {
            parameters.put("linkedTaskId", linkedTaskId);
        }
        return parameters;
    }

    private Map<String, Object> closeMetadata(String status, @Nullable Throwable failure)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("status", Thread.currentThread().isInterrupted() ? "aborted" : status);
        if (failure != null)
        {
            TraceFailureMetadata.addTo(metadata, failure, "Tool execution failed");
        }
        return metadata;
    }

    private String currentSkillName(LoomspanSession session)
    {
        try
        {
            return session.peekFrame().route();
        }
        catch (IllegalStateException ignored)
        {
            return session.getExecutionPlan().map(plan -> plan.capabilityName()).orElse("unknown");
        }
    }

}
