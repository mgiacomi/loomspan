package com.lokiscale.loomspan.internal.runtime.tool;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.core.TaskExecutionEvent;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.security.DefaultAccessGuard;
import com.lokiscale.loomspan.internal.vfs.RefResolver;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.ai.tool.ToolCallback;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.inOrder;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class ToolCallbackFactoryTest {

    @Test
    void recordsToolThrowableOnActiveFrameBeforeClosingIt() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        PlanningService planningService = mock(PlanningService.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        DefaultToolCallbackFactory factory = new DefaultToolCallbackFactory(router, planningService, stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-failure", "test.entry", 2);
        CapabilityMetadata capability = capability();
        ExecutionFrame toolFrame = new ExecutionFrame(
                "tool-frame-failure", null, com.lokiscale.loomspan.internal.core.OperationType.SKILL,
                TraceFrameType.TOOL_INVOCATION, capability.name(), java.util.Map.of(), Instant.parse("2026-03-15T12:00:00Z"));
        IllegalStateException failure = new IllegalStateException("tool boom");
        when(stateService.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any())).thenReturn(toolFrame);
        when(router.execute(eq(capability), any(), eq(session), eq(null))).thenThrow(failure);
        when(stateService.recordFailure(eq(session), eq(failure), any())).thenReturn("failure-1");

        ToolCallback callback = factory.createToolCallbacks(session, definitionWithEvidenceContract(), List.of(capability), null).getFirst();
        assertThatThrownBy(() -> callback.call("{\"value\":\"hello\"}")).hasCause(failure);

        org.mockito.InOrder order = inOrder(stateService);
        order.verify(stateService).recordFailure(eq(session), eq(failure), any());
        order.verify(stateService).closeFrame(eq(session), eq(toolFrame), any());
    }

    @Test
    void buildsVisibleToolDefinitions() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        PlanningService planningService = mock(PlanningService.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        DefaultToolCallbackFactory factory = new DefaultToolCallbackFactory(router, planningService, stateService);

        ToolCallback callback = factory.createToolCallbacks(
                com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2),
                definitionWithEvidenceContract(),
                List.of(capability()),
                null).getFirst();

        assertThat(callback.getToolDefinition().name()).isEqualTo("allowedVisibleSkill");
        assertThat(callback.getToolDefinition().description()).isEqualTo("child");
        assertThat(callback.getToolDefinition().inputSchema()).contains("\"type\" : \"object\"");
    }

    @Test
    void routesMappedExecutionsThroughPublicTraceIdentity() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        PlanningService planningService = mock(PlanningService.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        DefaultToolCallbackFactory factory = new DefaultToolCallbackFactory(router, planningService, stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
        CapabilityMetadata capability = capability();
        assertThat(capability.skillExecution().configured()).isFalse();
        assertThat(capability.name()).isEqualTo("allowedVisibleSkill");
        assertThat(capability.mappedTargetId()).isEqualTo("targetBean#deterministicTarget");
        ExecutionFrame toolFrame = new ExecutionFrame(
                "tool-frame-1",
                null,
                com.lokiscale.loomspan.internal.core.OperationType.SKILL,
                TraceFrameType.TOOL_INVOCATION,
                capability.name(),
                java.util.Map.of(),
                Instant.parse("2026-03-15T12:00:00Z"));
        ExecutionPlan linkedPlan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                com.lokiscale.loomspan.internal.core.PlanStatus.VALID,
                "task-1",
                List.of(new PlanTask("task-1", "Use tool", PlanTaskStatus.IN_PROGRESS, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, "Starting")));

        when(planningService.markToolStarted(eq(session), eq(capability), any())).thenReturn(Optional.of(linkedPlan));
        when(stateService.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any())).thenReturn(toolFrame);
        when(router.execute(eq(capability), any(), eq(session), eq(null))).thenReturn("child:hello");

        ToolCallback linkedCallback = factory.createToolCallbacks(session, definitionWithEvidenceContract(), List.of(capability), null).getFirst();
        Object linkedResult = linkedCallback.call("{\"value\":\"hello\"}");

        assertThat(linkedResult).isEqualTo("\"child:hello\"");
        verify(planningService).markToolCompleted(eq(session), eq("task-1"), eq(capability.name()), eq("child:hello"));
        org.mockito.InOrder inOrder = inOrder(stateService, router);
        inOrder.verify(stateService).openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any());
        inOrder.verify(stateService).logToolCall(eq(session), any());
        inOrder.verify(router).execute(eq(capability), any(), eq(session), eq(null));
        inOrder.verify(stateService).logToolResult(eq(session), any());
        inOrder.verify(stateService).closeFrame(eq(session), eq(toolFrame), any());

        when(planningService.markToolStarted(eq(session), eq(capability), any())).thenReturn(Optional.empty());
        when(stateService.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any())).thenReturn(toolFrame);
        when(router.execute(eq(capability), any(), eq(session), eq(null))).thenReturn("child:again");

        ToolCallback unplannedCallback = factory.createToolCallbacks(session, definitionWithEvidenceContract(), List.of(capability), null).getFirst();
        Object unplannedResult = unplannedCallback.call("{\"value\":\"again\"}");

        assertThat(unplannedResult).isEqualTo("\"child:again\"");
        verify(stateService).logUnplannedToolCall(eq(session), any());
        verify(stateService).recordSuccessfulSkill(eq(session), eq(capability.name()), eq(null), eq(true));
        verify(stateService, times(2)).closeFrame(eq(session), eq(toolFrame), any());

        ArgumentCaptor<TaskExecutionEvent> linkedCall = ArgumentCaptor.forClass(TaskExecutionEvent.class);
        ArgumentCaptor<TaskExecutionEvent> unplannedCall = ArgumentCaptor.forClass(TaskExecutionEvent.class);
        ArgumentCaptor<TaskExecutionEvent> results = ArgumentCaptor.forClass(TaskExecutionEvent.class);
        verify(stateService).logToolCall(eq(session), linkedCall.capture());
        verify(stateService).logUnplannedToolCall(eq(session), unplannedCall.capture());
        verify(stateService, times(2)).logToolResult(eq(session), results.capture());

        List<TaskExecutionEvent> traceEvents = new java.util.ArrayList<>();
        traceEvents.add(linkedCall.getValue());
        traceEvents.add(unplannedCall.getValue());
        traceEvents.addAll(results.getAllValues());
        assertThat(traceEvents).allSatisfy(event ->
        {
            assertThat(event.capabilityName()).isEqualTo(capability.name());
            assertThat(event.toString()).doesNotContain(capability.mappedTargetId());
        });
    }

    @Test
    void resolvesRefArgumentsBeforeDeterministicExecution() {
        PlanningService planningService = mock(PlanningService.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        RefResolver refResolver = (value, session) -> value instanceof String text && text.startsWith("ref://")
                ? "resolved-content"
                : value;
        CapabilityExecutionRouter router = new CapabilityExecutionRouter(
                refResolver,
                new StaticListableBeanFactory().getBeanProvider(com.lokiscale.loomspan.internal.core.ExecutionCoordinator.class),
                stateService,
                new DefaultAccessGuard());
        DefaultToolCallbackFactory factory = new DefaultToolCallbackFactory(router, planningService, stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);

        ToolCallback callback = factory.createToolCallbacks(session, definitionWithEvidenceContract(), List.of(capability()), null).getFirst();
        Object result = callback.call("{\"value\":\"ref://artifacts/input.txt\"}");

        assertThat(result).isEqualTo("\"child:resolved-content\"");
    }

    @Test
    void doesNotRecordEvidenceInsideCallbackForStepLoopBoundTaskExecutions() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        PlanningService planningService = mock(PlanningService.class);
        ExecutionStateService stateService = mock(ExecutionStateService.class);
        DefaultToolCallbackFactory factory = new DefaultToolCallbackFactory(router, planningService, stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
        CapabilityMetadata capability = capability();
        ExecutionFrame toolFrame = new ExecutionFrame(
                "tool-frame-1",
                null,
                com.lokiscale.loomspan.internal.core.OperationType.SKILL,
                TraceFrameType.TOOL_INVOCATION,
                capability.name(),
                java.util.Map.of(),
                Instant.parse("2026-03-15T12:00:00Z"));

        when(stateService.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any())).thenReturn(toolFrame);
        when(router.execute(eq(capability), any(), eq(session), eq(null))).thenReturn("child:hello");

        ToolCallback callback = factory.createToolCallbacks(session, definitionWithEvidenceContract(), List.of(capability), null).getFirst();
        Object result = callback.call(
                "{\"value\":\"hello\"}",
                new org.springframework.ai.chat.model.ToolContext(java.util.Map.of(
                        DefaultToolCallbackFactory.STEP_LOOP_TASK_ID_CONTEXT_KEY, "task-1")));

        assertThat(result).isEqualTo("\"child:hello\"");
        verify(planningService, never()).markToolCompleted(eq(session), eq("task-1"), eq(capability.name()), eq("child:hello"));
        verify(stateService, never()).recordSuccessfulSkill(eq(session), eq(capability.name()), eq("task-1"), eq(false));
    }

    private static CapabilityMetadata capability() {
        return new CapabilityMetadata(
                "yaml:child",
                "allowedVisibleSkill",
                "child",
                SkillExecutionDescriptor.none(),
                java.util.Set.of(),
                arguments -> "child:" + arguments.get("value"),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("allowedVisibleSkill", "child"),
                "targetBean#deterministicTarget");
    }

    private static com.lokiscale.loomspan.internal.skill.YamlSkillDefinition definitionWithEvidenceContract() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        return new com.lokiscale.loomspan.internal.skill.YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium"),
                com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(
                        java.util.Map.of("vendorName", "allowedVisibleSkill")));
    }
}
