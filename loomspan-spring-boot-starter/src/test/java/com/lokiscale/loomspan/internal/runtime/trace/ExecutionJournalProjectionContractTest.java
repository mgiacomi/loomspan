package com.lokiscale.loomspan.internal.runtime.trace;

import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaFailureMode;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationIssue;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class ExecutionJournalProjectionContractTest {

    private static final JsonMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    private final ExecutionJournalProjector projector = new ExecutionJournalProjector();

    @Test
    void projectsCanonicalDeveloperFacingJournalFromRepresentativeTraceStream() {
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-24T12:00:00Z"),
                List.of(new PlanTask("task-1", "Deploy", PlanTaskStatus.PENDING, null)));
        LinterOutcome linterOutcome = new LinterOutcome(
                "rootVisibleSkill",
                "regex",
                2,
                1,
                2,
                LinterOutcomeStatus.PASSED,
                "Return fenced YAML only.");
        OutputSchemaOutcome outputSchemaOutcome = new OutputSchemaOutcome(
                "rootVisibleSkill",
                OutputSchemaFailureMode.INVALID_JSON,
                2,
                1,
                2,
                OutputSchemaOutcomeStatus.EXHAUSTED,
                List.of(new OutputSchemaValidationIssue("$", "Response is not valid JSON.", "response")));

        List<JournalEntry> entries = projector.project(List.of(
                record(
                        1,
                        TraceRecordType.PLAN_CREATED,
                        "mission-frame",
                        "rootVisibleSkill",
                        Map.of("planId", "plan-1"),
                        plan),
                record(
                        2,
                        TraceRecordType.TOOL_CALL_STARTED,
                        "tool-frame",
                        "deploy.service",
                        Map.of("capabilityName", "deploy.service", "linkedTaskId", "task-1"),
                        Map.of("details", Map.of(
                                "arguments", Map.of(
                                        "target", "service-a",
                                        "token", "super-secret")))),
                record(
                        3,
                        TraceRecordType.TOOL_CALL_COMPLETED,
                        "tool-frame",
                        "deploy.service",
                        Map.of("capabilityName", "deploy.service", "linkedTaskId", "task-1"),
                        Map.of("details", Map.of("result", "ok"))),
                record(
                        4,
                        TraceRecordType.LINTER_RECORDED,
                        "mission-frame",
                        "rootVisibleSkill",
                        Map.of("skillName", "rootVisibleSkill", "status", "PASSED"),
                        linterOutcome),
                record(
                        5,
                        TraceRecordType.STRUCTURED_OUTPUT_RECORDED,
                        "mission-frame",
                        "rootVisibleSkill",
                        Map.of("skillName", "rootVisibleSkill", "status", "EXHAUSTED"),
                        outputSchemaOutcome),
                record(
                        6,
                        TraceRecordType.ERROR_RECORDED,
                        "mission-frame",
                        "rootVisibleSkill",
                        Map.of("exceptionType", "java.lang.IllegalStateException"),
                        Map.of("message", "boom", "apiKey", "top-secret")))).getEntriesSnapshot();

        assertThat(entries).extracting(JournalEntry::type)
                .containsExactly(
                        JournalEntryType.PLAN_CREATED,
                        JournalEntryType.TOOL_CALL,
                        JournalEntryType.TOOL_RESULT,
                        JournalEntryType.LINTER,
                        JournalEntryType.OUTPUT_SCHEMA,
                        JournalEntryType.ERROR);
        assertThat(entries).extracting(JournalEntry::frameId)
                .containsExactly("mission-frame", "tool-frame", "tool-frame", "mission-frame", "mission-frame", "mission-frame");
        assertThat(entries).extracting(JournalEntry::route)
                .containsExactly("rootVisibleSkill", "deploy.service", "deploy.service", "rootVisibleSkill", "rootVisibleSkill", "rootVisibleSkill");
        assertThat(entries.get(1).payload().get("details").get("arguments").get("token").textValue()).isEqualTo("[redacted]");
        assertThat(entries.get(1).payload().get("details").get("arguments").get("target").textValue()).isEqualTo("service-a");
        assertThat(entries.get(2).payload().get("details").get("result").textValue()).isEqualTo("ok");
        assertThat(entries.get(3).payload().get("status").textValue()).isEqualTo("PASSED");
        assertThat(entries.get(4).payload().get("status").textValue()).isEqualTo("EXHAUSTED");
        assertThat(entries.get(5).payload().get("message").textValue()).isEqualTo("boom");
        assertThat(entries.get(5).payload().get("exceptionType").textValue()).isEqualTo("java.lang.IllegalStateException");
    }

    @Test
    void ignoresRawTraceRecordsThatAreNotPartOfTheDeveloperFacingProjection() {
        List<JournalEntry> entries = projector.project(List.of(
                record(1, TraceRecordType.TRACE_STARTED, null, null, Map.of(), Map.of("sessionId", "session-1")),
                record(2, TraceRecordType.FRAME_OPENED, "frame-1", "rootVisibleSkill", Map.of(), Map.of("openedAt", "2026-03-24T12:00:00Z")),
                record(3, TraceRecordType.MODEL_REQUEST_PREPARED, "frame-1", "rootVisibleSkill", Map.of("segment", "mission"), Map.of("system", "do work")),
                record(4, TraceRecordType.MODEL_REQUEST_SENT, "frame-1", "rootVisibleSkill", Map.of("segment", "mission"), Map.of("objective", "hello")),
                record(5, TraceRecordType.MODEL_RESPONSE_RECEIVED, "frame-1", "rootVisibleSkill", Map.of("segment", "mission"), Map.of("content", "done")),
                record(6, TraceRecordType.FRAME_CLOSED, "frame-1", "rootVisibleSkill", Map.of("status", "completed"), Map.of("closedAt", "2026-03-24T12:00:01Z")),
                record(7, TraceRecordType.TRACE_COMPLETED, null, null, Map.of("errored", false), null)
        )).getEntriesSnapshot();

        assertThat(entries).isEmpty();
    }

    private static TraceRecord record(long sequence,
                                      TraceRecordType type,
                                      String frameId,
                                      String route,
                                      Map<String, Object> metadata,
                                      Object payload) {
        return new TraceRecord(
                "trace-1",
                "session-1",
                sequence,
                Instant.parse("2026-03-24T12:00:0" + Math.min(sequence, 9) + "Z"),
                type,
                frameId,
                null,
                null,
                route,
                "main",
                metadata,
                payload == null ? null : OBJECT_MAPPER.valueToTree(payload));
    }
}
