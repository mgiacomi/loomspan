package com.lokiscale.loomspan.internal.core;

import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class ExecutionJournalTest {

    private static final ObjectMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    @Test
    void deserializesOlderJournalsWithoutFrameId() throws Exception {
        String json = """
                {
                  "timestamp": "2026-03-15T12:00:00Z",
                  "level": "INFO",
                  "type": "THOUGHT",
                  "payload": "testing"
                }
                """;

        JournalEntry restored = OBJECT_MAPPER.readValue(json, JournalEntry.class);

        assertThat(restored.frameId()).isNull();
        assertThat(restored.route()).isNull();
        assertThat(restored.payload().textValue()).isEqualTo("testing");
    }

    @Test
    void roundTripsExecutionJournalThroughJackson() throws Exception {
        ExecutionJournal journal = new ExecutionJournal(List.of(
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:00Z"),
                        JournalLevel.INFO,
                        JournalEntryType.THOUGHT,
                        OBJECT_MAPPER.getNodeFactory().textNode("draft plan"),
                        null,
                        null),
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:01Z"),
                        JournalLevel.INFO,
                        JournalEntryType.TOOL_CALL,
                        OBJECT_MAPPER.valueToTree(Map.of("route", "tool.run", "arguments", Map.of("id", 42))),
                        "frame-1",
                        "tool.run")));

        String json = OBJECT_MAPPER.writeValueAsString(journal);
        ExecutionJournal restored = OBJECT_MAPPER.readValue(json, ExecutionJournal.class);

        assertThat(restored.getEntriesSnapshot()).containsExactlyElementsOf(journal.getEntriesSnapshot());
    }

    @Test
    void preservesFrameContextFromProjectedEntries() {
        ExecutionJournal journal = new ExecutionJournal(List.of(
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:00Z"),
                        JournalLevel.INFO,
                        JournalEntryType.THOUGHT,
                        OBJECT_MAPPER.getNodeFactory().textNode("draft plan"),
                        "frame-1",
                        "route.one")));

        assertThat(journal.getEntriesSnapshot()).singleElement().satisfies(entry -> {
            assertThat(entry.frameId()).isEqualTo("frame-1");
            assertThat(entry.route()).isEqualTo("route.one");
            assertThat(entry.payload().textValue()).isEqualTo("draft plan");
        });
    }

    @Test
    void supportsStructuredPayloadsWithoutCustomFixtureTypes() {
        ExecutionJournal journal = new ExecutionJournal(List.of(
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:00Z"),
                        JournalLevel.INFO,
                        JournalEntryType.THOUGHT,
                        OBJECT_MAPPER.getNodeFactory().textNode("draft plan"),
                        null,
                        null),
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:01Z"),
                        JournalLevel.INFO,
                        JournalEntryType.TOOL_RESULT,
                        OBJECT_MAPPER.valueToTree(Map.of("status", "ok", "result", Map.of("count", 2))),
                        null,
                        null)));

        assertThat(journal.getEntriesSnapshot().get(0).payload().isTextual()).isTrue();
        assertThat(journal.getEntriesSnapshot().get(0).payload().textValue()).isEqualTo("draft plan");
        assertThat(journal.getEntriesSnapshot().get(1).payload().isObject()).isTrue();
        assertThat(journal.getEntriesSnapshot().get(1).payload().get("status").textValue()).isEqualTo("ok");
        assertThat(journal.getEntriesSnapshot().get(1).payload().get("result").get("count").intValue()).isEqualTo(2);
    }

    @Test
    void serializesPlanSpecificJournalEntryTypesWithStructuredPayloads() {
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null),
                        new PlanTask("task-2", "Execute", PlanTaskStatus.IN_PROGRESS, "started")));
        ExecutionJournal journal = new ExecutionJournal(List.of(
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:00Z"),
                        JournalLevel.INFO,
                        JournalEntryType.PLAN_CREATED,
                        OBJECT_MAPPER.valueToTree(plan),
                        null,
                        null),
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:01Z"),
                        JournalLevel.INFO,
                        JournalEntryType.PLAN_UPDATED,
                        OBJECT_MAPPER.valueToTree(plan),
                        null,
                        null)));

        assertThat(journal.getEntriesSnapshot()).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.PLAN_CREATED, JournalEntryType.PLAN_UPDATED);
        assertThat(journal.getEntriesSnapshot().get(0).payload().get("tasks").get(1).get("status").textValue())
                .isEqualTo("IN_PROGRESS");
        assertThat(journal.getEntriesSnapshot().get(0).payload().get("tasks").get(1).get("note").textValue())
                .isEqualTo("started");
    }

    @Test
    void serializesLinterOutcomesWithStableStructuredFields() {
        LinterOutcome outcome = new LinterOutcome(
                "lintedSkill",
                "regex",
                3,
                2,
                2,
                LinterOutcomeStatus.EXHAUSTED,
                "Return fenced YAML only.");
        ExecutionJournal journal = new ExecutionJournal(List.of(
                new JournalEntry(
                        Instant.parse("2026-03-15T12:00:00Z"),
                        JournalLevel.INFO,
                        JournalEntryType.LINTER,
                        OBJECT_MAPPER.valueToTree(outcome),
                        null,
                        null)));

        assertThat(journal.getEntriesSnapshot().getFirst().payload().get("retryCount").intValue()).isEqualTo(2);
        assertThat(journal.getEntriesSnapshot().getFirst().payload().get("status").textValue()).isEqualTo("EXHAUSTED");
    }
}
