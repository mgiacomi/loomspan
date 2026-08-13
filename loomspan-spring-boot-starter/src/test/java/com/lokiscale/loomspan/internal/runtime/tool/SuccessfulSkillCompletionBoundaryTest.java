package com.lokiscale.loomspan.internal.runtime.tool;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.CancellationException;

import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.inOrder;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class SuccessfulSkillCompletionBoundaryTest
{
    @Test
    void linkedCallIsCreditedOnlyThroughVerifiedPlanCompletionAfterExecutionReturns()
    {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        PlanningService planning = mock(PlanningService.class);
        ExecutionStateService state = mock(ExecutionStateService.class);
        CapabilityMetadata capability = capability();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("linked", "test.entry", 3);
        when(planning.markToolStarted(eq(session), eq(capability), any())).thenReturn(Optional.of(linkedPlan()));
        when(state.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any()))
                .thenReturn(frame(capability.name()));
        when(router.execute(eq(capability), any(), eq(session), eq(null))).thenReturn("done");

        BoundCapability callback = new DefaultCapabilityInvoker(router, planning, state)
                .bind(session, definitionWithoutEvidenceContract(), List.of(capability), null)
                .getFirst();
        callback.invoke(Map.of(), null);

        org.mockito.InOrder order = inOrder(planning, router);
        order.verify(planning).markToolStarted(eq(session), eq(capability), any());
        order.verify(router).execute(eq(capability), any(), eq(session), eq(null));
        order.verify(planning).markToolCompleted(session, "task-1", capability.name(), "done");
        verify(state, never()).recordSuccessfulSkill(any(), any(), any(), any(Boolean.class));
    }

    @Test
    void failedAndCancelledUnplannedCallsNeverReceiveSuccessfulCredit()
    {
        for (RuntimeException failure : List.of(
                new IllegalStateException("failed"),
                new CancellationException("cancelled")))
        {
            CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
            PlanningService planning = mock(PlanningService.class);
            ExecutionStateService state = mock(ExecutionStateService.class);
            CapabilityMetadata capability = capability();
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "failure-" + failure.getClass().getSimpleName(), "test.entry", 3);
            when(planning.markToolStarted(eq(session), eq(capability), any())).thenReturn(Optional.empty());
            when(state.openFrame(eq(session), eq(TraceFrameType.TOOL_INVOCATION), eq(capability.name()), any()))
                    .thenReturn(frame(capability.name()));
            when(router.execute(eq(capability), any(), eq(session), eq(null))).thenThrow(failure);

            BoundCapability callback = new DefaultCapabilityInvoker(router, planning, state)
                    .bind(session, definitionWithoutEvidenceContract(), List.of(capability), null)
                    .getFirst();

            assertThatThrownBy(() -> callback.invoke(Map.of(), null)).isSameAs(failure);
            verify(state, never()).recordSuccessfulSkill(eq(session), eq(capability.name()), eq(null), eq(true));
            verify(planning, never()).markToolCompleted(any(), any(), any(), any());
        }
    }

    private static CapabilityMetadata capability()
    {
        return new CapabilityMetadata(
                "yaml:child",
                "investigateNetwork",
                "Investigate the network",
                SkillExecutionDescriptor.none(),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("investigateNetwork", "Investigate the network"),
                null);
    }

    private static YamlSkillDefinition definitionWithoutEvidenceContract()
    {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("handleIncident");
        manifest.setDescription("Handle incident");
        manifest.setModel("gpt-5");
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                new EffectiveSkillExecutionConfiguration(
                        "gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium"));
    }

    private static ExecutionPlan linkedPlan()
    {
        return new ExecutionPlan(
                "plan-1",
                "handleIncident",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                "task-1",
                List.of(new PlanTask(
                        "task-1", "Investigate", PlanTaskStatus.IN_PROGRESS, "investigateNetwork",
                        "Investigate", List.of(), List.of(), false, null)));
    }

    private static ExecutionFrame frame(String route)
    {
        return new ExecutionFrame(
                "frame-1", null, com.lokiscale.loomspan.internal.core.OperationType.SKILL,
                TraceFrameType.TOOL_INVOCATION, route, Map.of(), Instant.parse("2026-03-15T12:00:00Z"));
    }
}
