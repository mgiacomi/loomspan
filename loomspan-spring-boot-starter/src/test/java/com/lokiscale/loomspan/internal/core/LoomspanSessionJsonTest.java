package com.lokiscale.loomspan.internal.core;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.json.JsonMapper;
import org.junit.jupiter.api.Test;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class LoomspanSessionJsonTest {

    private static final JsonMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void serializesExecutionTraceAndDerivedJournalForLiveSession() throws Exception {
        LoomspanSession session = TestLoomspanSessions.withId(
                "session-json-live",
                "test.entry",
                4,
                null,
                TracePersistencePolicy.ALWAYS,
                FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                java.util.List.of());

        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:01Z"), Map.of("planId", plan.planId()), plan);

        JsonNode json = OBJECT_MAPPER.readTree(OBJECT_MAPPER.writeValueAsString(session));

        assertThat(json.get("sessionId").asText()).isEqualTo("session-json-live");
        assertThat(json.get("executionTrace").get("sessionId").asText()).isEqualTo("session-json-live");
        assertThat(json.get("executionTrace").get("traceId").asText()).isNotBlank();
        assertThat(json.get("executionTrace").get("filePath").asText()).contains("session-json-live");
        assertThat(json.get("executionTrace").get("persistencePolicy").asText()).isEqualTo("ALWAYS");
        assertThat(json.get("executionTrace").get("completed").asBoolean()).isFalse();
        assertThat(json.get("executionJournal").get("entries")).hasSize(1);
        assertThat(json.get("executionJournal").get("entries").get(0).get("type").asText()).isEqualTo("PLAN_CREATED");
        assertThat(json.has("executionTraceHandle")).isFalse();
        assertThat(json.has("authentication")).isFalse();
    }

    @Test
    void preservesDerivedJournalAfterFinalizationDeletesNeverRetainedTraceFile() throws Exception {
        LoomspanSession session = TestLoomspanSessions.withId(
                "session-json-finalized",
                "test.entry",
                4,
                null,
                TracePersistencePolicy.NEVER,
                FIXED_CLOCK);

        appendRecord(
                session,
                TraceRecordType.ERROR_RECORDED,
                Instant.parse("2026-03-15T12:00:02Z"),
                Map.of("exceptionType", "java.lang.IllegalStateException"),
                Map.of("message", "boom"));

        session.markTraceErrored();
        session.finalizeTrace(new TraceCompletion(
                TraceOutcome.FAILED,
                com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty(),
                "failure-json",
                Map.of()));

        JsonNode json = OBJECT_MAPPER.readTree(OBJECT_MAPPER.writeValueAsString(session));

        assertThat(json.get("executionTrace").get("completed").asBoolean()).isTrue();
        assertThat(json.get("executionTrace").get("errored").asBoolean()).isTrue();
        assertThat(json.get("executionTrace").get("filePath").isNull()).isTrue();
        assertThat(json.get("executionJournal").get("entries")).hasSize(1);
        assertThat(json.get("executionJournal").get("entries").get(0).get("type").asText()).isEqualTo("ERROR");
        assertThat(json.get("executionJournal").get("entries").get(0).get("payload").get("message").asText()).isEqualTo("boom");
    }

    private static void appendRecord(LoomspanSession session,
                                     TraceRecordType type,
                                     Instant timestamp,
                                     Map<String, Object> metadata,
                                     Object payload) {
        java.util.LinkedHashMap<String, Object> traceMetadata = new java.util.LinkedHashMap<>();
        if (metadata != null) {
            traceMetadata.putAll(metadata);
        }
        traceMetadata.put("timestampOverride", timestamp.toString());
        session.appendTraceRecord(type, traceMetadata, payload);
    }
}
