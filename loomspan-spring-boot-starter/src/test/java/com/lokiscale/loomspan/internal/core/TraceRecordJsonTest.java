package com.lokiscale.loomspan.internal.core;

import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.json.JsonMapper;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class TraceRecordJsonTest {

    private static final JsonMapper JSON = JsonMapper.builder()
            .findAndAddModules()
            .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES)
            .build();

    @Test
    void canonicalJsonRoundTripsWithoutEnvelopeSchemaVersion() throws Exception {
        TraceRecord record = new TraceRecord(
                "trace-1",
                "session-1",
                1,
                Instant.parse("2026-07-24T12:00:00Z"),
                TraceRecordType.TRACE_STARTED,
                null,
                null,
                null,
                null,
                "test",
                Map.of("capturePolicy", "ALWAYS"),
                null);

        String encoded = JSON.writeValueAsString(record);

        assertThat(encoded).doesNotContain("schemaVersion");
        assertThat(JSON.readValue(encoded, TraceRecord.class))
                .usingRecursiveComparison()
                .isEqualTo(record);
    }

    @Test
    void obsoleteSchemaVersionEnvelopeIsRejected() {
        String obsolete = """
                {"schemaVersion":1,"traceId":"trace-1","sessionId":"session-1","sequence":1,
                "timestamp":"2026-07-24T12:00:00Z","recordType":"TRACE_STARTED",
                "threadName":"test","metadata":{}}
                """;

        assertThatThrownBy(() -> JSON.readValue(obsolete, TraceRecord.class))
                .hasMessageContaining("schemaVersion");
    }
}
