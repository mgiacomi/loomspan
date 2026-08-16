package com.lokiscale.loomspan.internal.core;

import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ExecutionPlanTest {

    @Test
    void storesImmutableOrderedTasks() {
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null),
                        new PlanTask("task-2", "Execute", PlanTaskStatus.IN_PROGRESS, "started")));

        assertThat(plan.tasks()).extracting(PlanTask::taskId).containsExactly("task-1", "task-2");
        assertThatThrownBy(() -> plan.tasks().add(new PlanTask("task-3", "Done", PlanTaskStatus.COMPLETED, null)))
                .isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void updatesOnlyMatchingTask() {
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null),
                        new PlanTask("task-2", "Execute", PlanTaskStatus.PENDING, null)));

        ExecutionPlan updated = plan.updateTask("task-2",
                task -> task.withStatus(PlanTaskStatus.COMPLETED, "done"));

        assertThat(updated.tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.PENDING, PlanTaskStatus.COMPLETED);
        assertThat(updated.tasks().get(1).note()).isEqualTo("done");
    }

    @Test
    void allImmutablePlanCopiesPreservePlanId() {
        ExecutionPlan plan = new ExecutionPlan(
                "framework-plan-id",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null),
                        new PlanTask("task-2", "Execute", PlanTaskStatus.PENDING, null)));

        assertThat(plan.updateTask("task-2", task -> task.withStatus(PlanTaskStatus.COMPLETED, "done")).planId())
                .isEqualTo(plan.planId());
        assertThat(plan.withActiveTask("task-1").planId()).isEqualTo(plan.planId());
        assertThat(plan.withActiveTask("task-1").clearActiveTask().planId()).isEqualTo(plan.planId());
        assertThat(plan.withStatus(PlanStatus.STALE).planId()).isEqualTo(plan.planId());
    }
}
