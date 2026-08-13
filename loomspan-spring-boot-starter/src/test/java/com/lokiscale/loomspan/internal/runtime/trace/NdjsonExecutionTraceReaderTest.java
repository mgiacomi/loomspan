package com.lokiscale.loomspan.internal.runtime.trace;

import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class NdjsonExecutionTraceReaderTest {

    private static final JsonMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    @Test
    void reconstructsChunkedPayloadsByPayloadId() throws Exception {
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                "chunked-session",
                "test.entry",
                TracePersistencePolicy.ALWAYS,
                Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC));

        String largePayload = "x".repeat(10_000);
        handle.append(TraceRecordType.MODEL_RESPONSE_RECEIVED, java.util.Map.of("role", "assistant"), largePayload);

        List<TraceRecord> records = new ArrayList<>();
        handle.readRecords(records::add);
        TraceRecord record = records.stream()
                .filter(current -> current.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED)
                .findFirst()
                .orElseThrow();

        assertThat(record.metadata()).containsEntry("payloadChunked", true);
        assertThat(record.data().asText()).isEqualTo(largePayload);
        assertThat(records.stream().filter(current -> current.recordType() == TraceRecordType.PAYLOAD_CHUNK_APPENDED).count())
                .isGreaterThan(1);
    }

    @Test
    void reconstructsChunkedPayloadsUsingChunkIndexOrder() throws Exception {
        Path tracePath = Files.createTempFile("out-of-order-chunks", ".ndjson");
        try {
            TraceRecord envelope = new TraceRecord(
                    "trace-1",
                    "session-1",
                    1,
                    Instant.parse("2026-03-24T12:00:00Z"),
                    TraceRecordType.MODEL_RESPONSE_RECEIVED,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of(
                            "payloadId", "payload-1",
                            "chunkCount", 3,
                            "payloadChunked", true,
                            "contentType", "text/plain"),
                    null);
            TraceRecord chunk2 = new TraceRecord(
                    "trace-1",
                    "session-1",
                    4,
                    Instant.parse("2026-03-24T12:00:03Z"),
                    TraceRecordType.PAYLOAD_CHUNK_APPENDED,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of("payloadId", "payload-1", "chunkIndex", 2, "chunkCount", 3, "contentType", "text/plain"),
                    OBJECT_MAPPER.getNodeFactory().textNode("gamma"));
            TraceRecord chunk0 = new TraceRecord(
                    "trace-1",
                    "session-1",
                    2,
                    Instant.parse("2026-03-24T12:00:01Z"),
                    TraceRecordType.PAYLOAD_CHUNK_APPENDED,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of("payloadId", "payload-1", "chunkIndex", 0, "chunkCount", 3, "contentType", "text/plain"),
                    OBJECT_MAPPER.getNodeFactory().textNode("alpha"));
            TraceRecord chunk1 = new TraceRecord(
                    "trace-1",
                    "session-1",
                    3,
                    Instant.parse("2026-03-24T12:00:02Z"),
                    TraceRecordType.PAYLOAD_CHUNK_APPENDED,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of("payloadId", "payload-1", "chunkIndex", 1, "chunkCount", 3, "contentType", "text/plain"),
                    OBJECT_MAPPER.getNodeFactory().textNode("beta"));
            Files.writeString(
                    tracePath,
                    String.join(
                            System.lineSeparator(),
                            OBJECT_MAPPER.writeValueAsString(envelope),
                            OBJECT_MAPPER.writeValueAsString(chunk2),
                            OBJECT_MAPPER.writeValueAsString(chunk0),
                            OBJECT_MAPPER.writeValueAsString(chunk1))
                            + System.lineSeparator(),
                    StandardCharsets.UTF_8);

            List<TraceRecord> records = new ArrayList<>();
            new NdjsonExecutionTraceReader().read(tracePath, records::add);

            assertThat(records.getFirst().recordType()).isEqualTo(TraceRecordType.MODEL_RESPONSE_RECEIVED);
            assertThat(records.getFirst().data().asText()).isEqualTo("alphabetagamma");
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void toleratesIncompleteChunkedPayloadsWhileReadingActiveTrace() throws Exception {
        Path tracePath = Files.createTempFile("partial-trace", ".ndjson");
        try {
            TraceRecord envelope = new TraceRecord(
                    "trace-1",
                    "session-1",
                    1,
                    Instant.parse("2026-03-24T12:00:00Z"),
                    TraceRecordType.MODEL_RESPONSE_RECEIVED,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of(
                            "payloadId", "payload-1",
                            "chunkCount", 3,
                            "payloadChunked", true,
                            "contentType", "text/plain"),
                    null);
            Files.writeString(tracePath, OBJECT_MAPPER.writeValueAsString(envelope) + System.lineSeparator());

            List<TraceRecord> records = new ArrayList<>();
            new NdjsonExecutionTraceReader().read(tracePath, records::add);

            assertThat(records).hasSize(1);
            assertThat(records.getFirst().recordType()).isEqualTo(TraceRecordType.MODEL_RESPONSE_RECEIVED);
            assertThat(records.getFirst().metadata()).containsEntry("payloadChunked", true);
            assertThat(records.getFirst().metadata()).containsEntry("payloadId", "payload-1");
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void ignoresTrailingPartialRecordDuringActiveRead() throws Exception {
        Path tracePath = Files.createTempFile("partial-line-trace", ".ndjson");
        try {
            TraceRecord completeRecord = new TraceRecord(
                    "trace-1",
                    "session-1",
                    1,
                    Instant.parse("2026-03-24T12:00:00Z"),
                    TraceRecordType.MODEL_REQUEST_SENT,
                    "frame-1",
                    null,
                    null,
                    "rootVisibleSkill",
                    "main",
                    java.util.Map.of(),
                    OBJECT_MAPPER.getNodeFactory().textNode("ok"));
            Files.writeString(
                    tracePath,
                    OBJECT_MAPPER.writeValueAsString(completeRecord) + System.lineSeparator() + "{\"traceId\":\"trace-1\"",
                    StandardCharsets.UTF_8);

            List<TraceRecord> records = new ArrayList<>();
            new NdjsonExecutionTraceReader().read(tracePath, records::add);

            assertThat(records).hasSize(1);
            assertThat(records.getFirst().recordType()).isEqualTo(TraceRecordType.MODEL_REQUEST_SENT);
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void streamsManyRecordsWithoutRequiringWholeFileMaterialization() throws Exception {
        Path tracePath = Files.createTempFile("streaming-trace", ".ndjson");
        try {
            StringBuilder content = new StringBuilder();
            for (int index = 0; index < 250; index++) {
                TraceRecord record = new TraceRecord(
                        "trace-1",
                        "session-1",
                        index + 1L,
                        Instant.parse("2026-03-24T12:00:00Z").plusSeconds(index),
                        TraceRecordType.MODEL_REQUEST_SENT,
                        "frame-" + index,
                        null,
                        null,
                        "rootVisibleSkill",
                        "main",
                        java.util.Map.of("index", index),
                        OBJECT_MAPPER.getNodeFactory().textNode("payload-" + index));
                content.append(OBJECT_MAPPER.writeValueAsString(record)).append(System.lineSeparator());
            }
            Files.writeString(tracePath, content, StandardCharsets.UTF_8);

            List<TraceRecord> records = new ArrayList<>();
            new NdjsonExecutionTraceReader().read(tracePath, records::add);

            assertThat(records).hasSize(250);
            assertThat(records.getFirst().data().asText()).isEqualTo("payload-0");
            assertThat(records.getLast().data().asText()).isEqualTo("payload-249");
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }
}
