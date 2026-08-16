package com.lokiscale.loomspan.internal.runtime.state;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.ExecutionTraceRecorder;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.TaskExecutionEvent;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaFailureMode;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationIssue;
import org.junit.jupiter.api.Test;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ExecutionStateServiceTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void successfulSkillSnapshotDefensivelyPreservesInsertionOrder() {
        LinkedHashSet<String> source = new LinkedHashSet<>(List.of("invoiceParser", "expenseLookup"));
        SuccessfulSkillSnapshot snapshot = new SuccessfulSkillSnapshot(source);
        source.add("taxLookup");

        assertThat(snapshot.successfulDirectSkills()).containsExactly("invoiceParser", "expenseLookup");
        assertThatThrownBy(() -> snapshot.successfulDirectSkills().add("taxLookup"))
                .isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void managesFramePlanAndJournalWritesThroughSingleBoundary() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 3);
        ExecutionPlan plan = plan("plan-1");
        LinterOutcome outcome = new LinterOutcome(
                "lintedSkill",
                "regex",
                2,
                1,
                2,
                LinterOutcomeStatus.PASSED,
                "Return fenced YAML only.");
        OutputSchemaOutcome outputSchemaOutcome = new OutputSchemaOutcome(
                "schema.skill",
                OutputSchemaFailureMode.SCHEMA_VALIDATION_FAILED,
                1,
                0,
                2,
                OutputSchemaOutcomeStatus.RETRYING,
                List.of(new OutputSchemaValidationIssue("$.vendorName", "missing required field 'vendorName'", "vendorName")));

        ExecutionFrame frame = stateService.openMissionFrame(session, "rootVisibleSkill", Map.of("objective", "hello"));
        stateService.storePlan(session, plan);
        stateService.logPlanCreated(session, plan, Map.of(
                "attemptId", "attempt-plan-created",
                "retrySequenceId", "retry-plan-created"));
        stateService.logToolCall(session, TaskExecutionEvent.linked("allowedVisibleSkill", "task-1", Map.of("arguments", Map.of("value", "hello")), null));
        stateService.logToolResult(session, TaskExecutionEvent.linked("allowedVisibleSkill", "task-1", Map.of("result", "done"), null));
        stateService.recordLinterOutcome(session, outcome);
        stateService.recordOutputSchemaOutcome(session, outputSchemaOutcome);
        stateService.recordFailure(session, new IllegalStateException("boom"), Map.of("message", "boom"));
        stateService.closeMissionFrame(session, frame);
        stateService.clearPlan(session);

        assertThat(session.getFramesSnapshot()).isEmpty();
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(session.getLastLinterOutcome()).contains(outcome);
        assertThat(session.getLastOutputSchemaOutcome()).contains(outputSchemaOutcome);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(
                        JournalEntryType.PLAN_CREATED,
                        JournalEntryType.TOOL_CALL,
                        JournalEntryType.TOOL_RESULT,
                        JournalEntryType.LINTER,
                        JournalEntryType.OUTPUT_SCHEMA,
                        JournalEntryType.ERROR);
        assertThat(session.getJournalSnapshot().get(3).payload().get("status").textValue()).isEqualTo("PASSED");
        assertThat(session.getJournalSnapshot().get(4).payload().get("failureMode").textValue()).isEqualTo("SCHEMA_VALIDATION_FAILED");
    }

    @Test
    void restoresParentPlanAfterNestedMissionAndClearsWhenNoParentExists() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);

        LoomspanSession sessionWithParent = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-parent", "test.entry", 3);
        ExecutionPlan parentPlan = plan("parent-plan");
        stateService.storePlan(sessionWithParent, parentPlan);
        PlanSnapshot snapshot = stateService.snapshotPlan(sessionWithParent);
        ExecutionPlan childPlan = plan("child-plan");
        stateService.storePlan(sessionWithParent, childPlan);
        assertThat(sessionWithParent.getExecutionPlan()).get().extracting(ExecutionPlan::planId).isEqualTo("child-plan");
        stateService.restorePlan(sessionWithParent, snapshot);
        ExecutionPlan updatedParent = sessionWithParent.getExecutionPlan().orElseThrow().withStatus(PlanStatus.STALE);
        stateService.storePlan(sessionWithParent, updatedParent);

        LoomspanSession sessionWithoutParent = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-empty", "test.entry", 3);
        PlanSnapshot emptySnapshot = stateService.snapshotPlan(sessionWithoutParent);
        stateService.storePlan(sessionWithoutParent, plan("child-plan"));
        stateService.restorePlan(sessionWithoutParent, emptySnapshot);

        assertThat(sessionWithParent.getExecutionPlan()).contains(updatedParent);
        assertThat(updatedParent.planId()).isEqualTo(parentPlan.planId()).isNotEqualTo(childPlan.planId());
        assertThat(sessionWithoutParent.getExecutionPlan()).isEmpty();
    }

    @Test
    void recordsAcceptedAttemptOnPlanCreationButNotPlanUpdates() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-plan-record-contract", "test.entry", 3);
        ExecutionPlan plan = plan("framework-plan-id");
        stateService.openMissionFrame(session, "rootVisibleSkill", Map.of());

        stateService.logPlanCreated(session, plan, Map.of(
                "attemptId", "attempt-accepted",
                "retrySequenceId", "retry-sequence",
                "attemptNumber", 7,
                "untrustedExtra", "must-not-copy"));
        ExecutionPlan updated = plan.withStatus(PlanStatus.STALE);
        stateService.logPlanUpdated(session, updated);

        List<TraceRecord> planRecords = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.PLAN_CREATED
                        || record.recordType() == TraceRecordType.PLAN_UPDATED)
                .toList();
        assertThat(planRecords).hasSize(2);
        TraceRecord created = planRecords.get(0);
        TraceRecord update = planRecords.get(1);
        assertThat(created.metadata())
                .containsEntry("planId", "framework-plan-id")
                .containsEntry("attemptId", "attempt-accepted")
                .containsEntry("retrySequenceId", "retry-sequence")
                .containsEntry("recordedAt", "2026-03-15T12:00:00Z")
                .doesNotContainKeys("attemptNumber", "untrustedExtra");
        assertThat(created.data().path("planId").asText()).isEqualTo("framework-plan-id");
        assertThat(update.metadata())
                .containsEntry("planId", "framework-plan-id")
                .doesNotContainKeys("attemptId", "retrySequenceId");
        assertThat(update.data().path("planId").asText()).isEqualTo("framework-plan-id");
    }

    @Test
    void restoresParentSuccessfulSkillsAfterNestedMissionAndClearsWhenNoParentExists() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);

        LoomspanSession sessionWithParent = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-parent-evidence", "test.entry", 3);
        stateService.recordSuccessfulSkill(sessionWithParent, "invoiceParser", "task-1", false);
        stateService.recordSuccessfulSkill(sessionWithParent, "expenseLookup", "task-2", false);
        SuccessfulSkillSnapshot snapshot = stateService.snapshotSuccessfulSkills(sessionWithParent);
        assertThat(snapshot.successfulDirectSkills()).containsExactly("invoiceParser", "expenseLookup");
        stateService.recordSuccessfulSkill(sessionWithParent, "taxLookup", "task-3", false);
        stateService.restoreSuccessfulSkills(sessionWithParent, snapshot);

        LoomspanSession sessionWithoutParent = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-empty-evidence", "test.entry", 3);
        SuccessfulSkillSnapshot emptySnapshot = stateService.snapshotSuccessfulSkills(sessionWithoutParent);
        stateService.recordSuccessfulSkill(sessionWithoutParent, "expenseLookup", "task-2", false);
        stateService.restoreSuccessfulSkills(sessionWithoutParent, emptySnapshot);

        assertThat(sessionWithParent.getSuccessfulDirectSkills()).containsExactly("invoiceParser", "expenseLookup");
        assertThat(sessionWithoutParent.getSuccessfulDirectSkills()).isEmpty();
    }

    @Test
    void rejectsClosingFrameOutOfOrder() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-frames", "test.entry", 3);

        ExecutionFrame parentFrame = stateService.openMissionFrame(session, "rootVisibleSkill", Map.of());
        ExecutionFrame childFrame = stateService.openMissionFrame(session, "child.visible.skill", Map.of());

        assertThatThrownBy(() -> stateService.closeMissionFrame(session, parentFrame))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining(parentFrame.frameId());
        assertThat(session.peekFrame()).isEqualTo(childFrame);
        assertThat(session.getFramesSnapshot()).containsExactly(childFrame, parentFrame);
    }

    @Test
    void recordsRuntimeTraceEventsAgainstTheActiveFrameAndIncludesCanonicalToolStartAndRootMissionTyping() throws Exception {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-trace", "test.entry", 3);

        ExecutionFrame rootFrame = stateService.openMissionFrame(session, "rootVisibleSkill", Map.of("objective", "hello"));
        ExecutionFrame frame = stateService.openFrame(session, TraceFrameType.MODEL_CALL, "rootVisibleSkill#model", Map.of("driver", "openai"));
        stateService.recordModelRequestPrepared(
                session,
                frame,
                new ModelTraceContext(new com.lokiscale.loomspan.internal.core.ModelExecutionIdentity(
                        "gpt-5", "openai-main", com.lokiscale.loomspan.autoconfigure.AiDriver.OPENAI,
                        "openai/gpt-5"), "rootVisibleSkill", "unit"),
                Map.of("retrySequenceId", "sequence-1", "attemptId", "attempt-1", "attemptNumber", 1,
                        "attemptReason", "INITIAL", "providerAttemptNumber", 1),
                Map.of("user", "hello"));
        stateService.logToolCall(session, TaskExecutionEvent.linked("allowedVisibleSkill", "task-1", Map.of("arguments", Map.of("value", "hello")), null));
        stateService.closeFrame(session, frame, Map.of("status", "completed"));
        stateService.closeMissionFrame(session, rootFrame);

        List<TraceRecord> records = readRecords(session);

        TraceRecord frameOpened = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_OPENED && record.frameId().equals(rootFrame.frameId()))
                .findFirst()
                .orElseThrow();
        TraceRecord modelRequest = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_PREPARED)
                .findFirst()
                .orElseThrow();
        TraceRecord toolStarted = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.TOOL_CALL_STARTED)
                .findFirst()
                .orElseThrow();
        TraceRecord frameClosed = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                .findFirst()
                .orElseThrow();

        assertThat(frameOpened.frameId()).isEqualTo(rootFrame.frameId());
        assertThat(frameOpened.parentFrameId()).isNull();
        assertThat(frameOpened.frameType()).isEqualTo(TraceFrameType.ROOT_MISSION);
        assertThat(modelRequest.frameId()).isEqualTo(frame.frameId());
        assertThat(modelRequest.route()).isEqualTo("rootVisibleSkill#model");
        assertThat(toolStarted.frameId()).isEqualTo(frame.frameId());
        assertThat(toolStarted.recordType()).isEqualTo(TraceRecordType.TOOL_CALL_STARTED);
        assertThat(frameClosed.frameId()).isEqualTo(frame.frameId());
    }

    @Test
    void recordsOneCanonicalStartForPlannedToolCall() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-canonical-tool-start", "test.entry", 3);

        stateService.openMissionFrame(session, "rootVisibleSkill", Map.of());
        stateService.logToolCall(session, TaskExecutionEvent.linked(
                "allowedVisibleSkill",
                "task-1",
                Map.of("arguments", Map.of("value", "hello")),
                "Invoke the planned capability."));

        List<TraceRecord> toolRecords = readRecords(session).stream()
                .filter(record -> record.recordType().name().startsWith("TOOL_CALL_"))
                .toList();

        assertThat(toolRecords).singleElement().satisfies(record -> {
            assertThat(record.recordType()).isEqualTo(TraceRecordType.TOOL_CALL_STARTED);
            assertThat(record.data().path("eventId").asText()).isNotBlank();
            assertThat(record.data().path("capabilityName").asText()).isEqualTo("allowedVisibleSkill");
            assertThat(record.data().path("linkedTaskId").asText()).isEqualTo("task-1");
            assertThat(record.data().path("details").path("arguments").path("value").asText()).isEqualTo("hello");
            assertThat(record.data().path("note").asText()).isEqualTo("Invoke the planned capability.");
            assertThat(record.metadata().containsKey("unplanned")).isFalse();
        });
    }

    @Test
    void recordsOneCanonicalStartForUnplannedToolCall() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-unplanned-tool-start", "test.entry", 3);

        stateService.openMissionFrame(session, "rootVisibleSkill", Map.of());
        stateService.logUnplannedToolCall(session, new TaskExecutionEvent(
                "event-unplanned",
                "allowedVisibleSkill",
                null,
                Map.of("arguments", Map.of("value", "hello")),
                "No unique ready task matched this tool call"));

        assertThat(readRecords(session).stream()
                .filter(record -> record.recordType().name().startsWith("TOOL_CALL_")))
                .singleElement().satisfies(record -> {
                    assertThat(record.recordType()).isEqualTo(TraceRecordType.TOOL_CALL_STARTED);
                    assertThat(record.data().path("eventId").asText()).isEqualTo("event-unplanned");
                    assertThat(record.data().path("linkedTaskId").isNull()).isTrue();
                    assertThat(record.data().path("details").path("arguments").path("value").asText()).isEqualTo("hello");
                    assertThat(record.data().path("note").asText()).isEqualTo("No unique ready task matched this tool call");
                    assertThat(record.metadata()).containsEntry("unplanned", true).doesNotContainKey("linkedTaskId");
                });
    }

    @Test
    void recordsSuccessfulSkillInLedgerAndTraceWithoutJournalEntries() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-evidence", "test.entry", 3);

        ExecutionFrame rootFrame = stateService.openMissionFrame(session, "rootVisibleSkill", Map.of());
        stateService.recordSuccessfulSkill(session, "invoiceParser", "task-1", false);
        stateService.recordEvidenceValidation(session, false, Map.of("skillName", "rootVisibleSkill"), Map.of("claims", List.of("isDuplicate")));
        stateService.closeMissionFrame(session, rootFrame);

        assertThat(session.getSuccessfulDirectSkills()).containsExactly("invoiceParser");
        assertThat(session.getJournalSnapshot()).isEmpty();
        TraceRecord evidenceRecord = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.EVIDENCE_RECORDED)
                .findFirst()
                .orElseThrow();
        assertThat(evidenceRecord.data().path("successfulSkill").asText()).isEqualTo("invoiceParser");
        assertThat(evidenceRecord.data().path("successfulDirectSkills")).hasSize(1);
        assertThat(evidenceRecord.data().has("evidenceTypes")).isFalse();
        assertThat(evidenceRecord.data().has("ledger")).isFalse();
        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.EVIDENCE_VALIDATION_FAILED);
    }

    @Test
    void rollsBackFramePushWhenRecorderFailsDuringOpen() {
        ExecutionTraceRecorder failingRecorder = new ExecutionTraceRecorder() {
            @Override
            public void recordFrameOpened(LoomspanSession session, ExecutionFrame frame) {
                throw new IllegalStateException("boom");
            }

            @Override
            public void recordFrameClosed(LoomspanSession session, ExecutionFrame frame, Map<String, Object> metadata) {
            }

            @Override
            public void recordModelRequestPrepared(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
                    Map<String, Object> attempt, Object payload) {
            }

            @Override
            public void recordModelRequestSent(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
                    Map<String, Object> attempt, Object payload) {
            }

            @Override
            public void recordModelResponseReceived(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
                    Map<String, Object> attempt,
                    com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord usage,
                    Object payload) {
            }

            @Override
            public void recordModelAttemptFailed(LoomspanSession session, ExecutionFrame frame, ModelTraceContext context,
                    Map<String, Object> attempt, Map<String, Object> failureMetadata, Object payload) {
            }

            @Override
            public void recordPlanCreated(LoomspanSession session, ExecutionPlan plan, Map<String, Object> acceptedAttempt) {
            }

            @Override
            public void recordPlanUpdated(LoomspanSession session, ExecutionPlan plan) {
            }

            @Override
            public void recordToolStarted(LoomspanSession session, ExecutionFrame frame, com.lokiscale.loomspan.internal.core.ToolTraceContext context, Object payload) {
            }

            @Override
            public void recordToolCompleted(LoomspanSession session, ExecutionFrame frame, com.lokiscale.loomspan.internal.core.ToolTraceContext context, Object payload) {
            }

            @Override
            public void recordToolFailed(LoomspanSession session, ExecutionFrame frame, com.lokiscale.loomspan.internal.core.ToolTraceContext context, Object payload) {
            }

            @Override
            public void recordAdvisorRequestMutation(LoomspanSession session, com.lokiscale.loomspan.internal.core.AdvisorTraceContext context, Object payload) {
            }

            @Override
            public void recordAdvisorResponseMutation(LoomspanSession session, com.lokiscale.loomspan.internal.core.AdvisorTraceContext context, Object payload) {
            }

            @Override
            public void recordLinterOutcome(LoomspanSession session, LinterOutcome outcome) {
            }

            @Override
            public void recordOutputSchemaOutcome(LoomspanSession session, OutputSchemaOutcome outcome) {
            }

            @Override
            public void finalizeTrace(LoomspanSession session, com.lokiscale.loomspan.internal.core.TraceCompletion completion) {
            }
        };
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK, new com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService(), failingRecorder);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-open-failure", "test.entry", 3);

        assertThatThrownBy(() -> stateService.openMissionFrame(session, "rootVisibleSkill", Map.of("objective", "hello")))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("boom");

        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    private static ExecutionPlan plan(String planId) {
        return new ExecutionPlan(
                planId,
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool", PlanTaskStatus.PENDING, null)));
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }
}
