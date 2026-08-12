package com.lokiscale.loomspan.internal.runtime.trace;

import com.fasterxml.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class ExecutionJournalProjectorTest {

    private static final JsonMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    @Test
    void derivesSanitizedDeveloperFacingJournalFromTrace() {
        ExecutionJournalProjector projector = new ExecutionJournalProjector();
        TraceRecord toolCall = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-03-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_STARTED,
                "frame-1",
                null,
                null,
                "rootVisibleSkill",
                "main",
                Map.of("capabilityName", "deploy.service", "linkedTaskId", "task-1"),
                OBJECT_MAPPER.valueToTree(Map.of(
                        "details", Map.of(
                                "arguments", Map.of(
                                        "Authorization", "Bearer super-secret",
                                        "token", "secret-value",
                                        "target", "service-a")))));
        TraceRecord error = new TraceRecord(
                "trace-1",
                "session-1",
                2,
                Instant.parse("2026-03-24T12:00:01Z"),
                TraceRecordType.ERROR_RECORDED,
                "frame-1",
                null,
                null,
                "rootVisibleSkill",
                "main",
                Map.of("exceptionType", "java.lang.IllegalStateException"),
                OBJECT_MAPPER.valueToTree(Map.of("message", "boom", "apiKey", "top-secret")));

        List<JournalEntry> entries = projector.project(List.of(toolCall, error)).getEntriesSnapshot();

        assertThat(entries).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.TOOL_CALL, JournalEntryType.ERROR);
        assertThat(entries.getFirst().payload().get("capabilityName").textValue()).isEqualTo("deploy.service");
        assertThat(entries.getFirst().payload().get("details").get("arguments").get("Authorization").textValue()).isEqualTo("[redacted]");
        assertThat(entries.getFirst().payload().get("details").get("arguments").get("token").textValue()).isEqualTo("[redacted]");
        assertThat(entries.getFirst().payload().get("details").get("arguments").get("target").textValue()).isEqualTo("service-a");
        assertThat(entries.get(1).payload().get("message").textValue()).isEqualTo("boom");
        assertThat(entries.get(1).payload().get("sourceRecordType").textValue()).isEqualTo(TraceRecordType.ERROR_RECORDED.name());
        assertThat(entries.get(1).payload().get("exceptionType").textValue()).isEqualTo("java.lang.IllegalStateException");
    }

    @Test
    void preservesDistinctToolFailureAndErrorRecords() {
        ExecutionJournalProjector projector = new ExecutionJournalProjector();
        TraceRecord toolFailure = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-03-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_FAILED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of(
                        "capabilityName", "deploy.service",
                        "linkedTaskId", "task-1",
                        "message", "boom",
                        "exceptionType", "java.lang.IllegalStateException"),
                OBJECT_MAPPER.valueToTree(Map.of("arguments", Map.of("target", "service-a"))));
        TraceRecord genericError = new TraceRecord(
                "trace-1",
                "session-1",
                2,
                Instant.parse("2026-03-24T12:00:01Z"),
                TraceRecordType.ERROR_RECORDED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of(),
                OBJECT_MAPPER.valueToTree(Map.of(
                        "tool", "deploy.service",
                        "linkedTaskId", "task-1",
                        "message", "boom",
                        "exceptionType", "java.lang.IllegalStateException")));

        List<JournalEntry> entries = projector.project(List.of(toolFailure, genericError)).getEntriesSnapshot();

        assertThat(entries).hasSize(2);
        assertThat(entries).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.TOOL_FAILURE, JournalEntryType.ERROR);
        assertThat(entries.getFirst().payload().get("tool").textValue()).isEqualTo("deploy.service");
        assertThat(entries.getFirst().payload().get("message").textValue()).isEqualTo("boom");
        assertThat(entries.getFirst().payload().get("sourceRecordType").textValue()).isEqualTo(TraceRecordType.TOOL_CALL_FAILED.name());
        assertThat(entries.getFirst().payload().get("details").get("arguments").get("target").textValue()).isEqualTo("service-a");
        assertThat(entries.get(1).payload().get("tool").textValue()).isEqualTo("deploy.service");
        assertThat(entries.get(1).payload().get("message").textValue()).isEqualTo("boom");
        assertThat(entries.get(1).payload().get("sourceRecordType").textValue()).isEqualTo(TraceRecordType.ERROR_RECORDED.name());
    }

    @Test
    void surfacesNestedToolFailureSummaryFromTracePayload() {
        ExecutionJournalProjector projector = new ExecutionJournalProjector();
        TraceRecord toolFailure = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-03-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_FAILED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of("capabilityName", "deploy.service"),
                OBJECT_MAPPER.valueToTree(Map.of(
                        "arguments", Map.of("token", "secret-token"),
                        "failure", Map.of(
                                "linkedTaskId", "task-1",
                                "message", "router exploded",
                                "exceptionType", "java.lang.IllegalStateException",
                                "authorization", "Bearer abc"))));

        List<JournalEntry> entries = projector.project(List.of(toolFailure)).getEntriesSnapshot();

        assertThat(entries).singleElement().extracting(JournalEntry::type).isEqualTo(JournalEntryType.TOOL_FAILURE);
        assertThat(entries.getFirst().payload().get("tool").textValue()).isEqualTo("deploy.service");
        assertThat(entries.getFirst().payload().get("linkedTaskId").textValue()).isEqualTo("task-1");
        assertThat(entries.getFirst().payload().get("message").textValue()).isEqualTo("router exploded");
        assertThat(entries.getFirst().payload().get("exceptionType").textValue()).isEqualTo("java.lang.IllegalStateException");
        assertThat(entries.getFirst().payload().get("details").get("authorization").textValue()).isEqualTo("[redacted]");
    }

    @Test
    void preservesRepeatedLegitimateJournalEvents() {
        ExecutionJournalProjector projector = new ExecutionJournalProjector();
        TraceRecord firstToolResult = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-03-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_COMPLETED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of("capabilityName", "deploy.service", "linkedTaskId", "task-1"),
                OBJECT_MAPPER.valueToTree(Map.of("details", Map.of("result", "ok"))));
        TraceRecord secondToolResult = new TraceRecord(
                "trace-1",
                "session-1",
                2,
                Instant.parse("2026-03-24T12:00:01Z"),
                TraceRecordType.TOOL_CALL_COMPLETED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of("capabilityName", "deploy.service", "linkedTaskId", "task-1"),
                OBJECT_MAPPER.valueToTree(Map.of("details", Map.of("result", "ok"))));

        List<JournalEntry> entries = projector.project(List.of(firstToolResult, secondToolResult)).getEntriesSnapshot();

        assertThat(entries).hasSize(2);
        assertThat(entries).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.TOOL_RESULT, JournalEntryType.TOOL_RESULT);
    }

    @Test
    void doesNotInferUnplannedToolCallsFromLegacyMessageText() {
        ExecutionJournalProjector projector = new ExecutionJournalProjector();
        TraceRecord toolCall = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-03-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_STARTED,
                "frame-1",
                null,
                null,
                "deploy.service",
                "main",
                Map.of("capabilityName", "deploy.service"),
                OBJECT_MAPPER.valueToTree(Map.of("message", "No unique ready task matched this tool call")));

        List<JournalEntry> entries = projector.project(List.of(toolCall)).getEntriesSnapshot();

        assertThat(entries).singleElement().extracting(JournalEntry::type).isEqualTo(JournalEntryType.TOOL_CALL);
    }
}
