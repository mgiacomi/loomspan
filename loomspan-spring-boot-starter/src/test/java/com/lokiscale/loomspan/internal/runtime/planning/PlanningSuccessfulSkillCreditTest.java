package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import org.junit.jupiter.api.Test;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class PlanningSuccessfulSkillCreditTest
{
    @Test
    void startedAndFailedTasksDoNotCreditButVerifiedCompletionDoesExactlyOnce()
    {
        DefaultExecutionStateService state = new DefaultExecutionStateService(Clock.fixed(
                Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC));
        DefaultPlanningService planning = new DefaultPlanningService(new DefaultPlanTaskLinker(), state);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("planning-credit", "test.entry", 3);
        state.storePlan(session, plan("task-success", "investigateNetwork"));

        planning.markTaskStarted(session, "task-success", "investigateNetwork", java.util.Map.of()).orElseThrow();
        assertThat(session.getSuccessfulDirectSkills()).isEmpty();
        planning.markToolCompleted(session, "task-success", "investigateNetwork", "done").orElseThrow();
        assertThat(session.getSuccessfulDirectSkills()).containsExactly("investigateNetwork");

        state.storePlan(session, plan("task-failure", "investigateApp"));
        planning.markTaskStarted(session, "task-failure", "investigateApp", java.util.Map.of()).orElseThrow();
        planning.markToolFailed(session, "task-failure", "investigateApp", new IllegalStateException("failed"));
        assertThat(session.getSuccessfulDirectSkills())
                .containsExactly("investigateNetwork")
                .doesNotContain("investigateApp");
    }

    private static ExecutionPlan plan(String taskId, String capability)
    {
        return new ExecutionPlan(
                "plan-" + taskId,
                "handleIncident",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask(
                        taskId,
                        "Use " + capability,
                        PlanTaskStatus.PENDING,
                        capability,
                        "Use capability",
                        List.of(),
                        List.of(),
                        false,
                        null)));
    }
}
