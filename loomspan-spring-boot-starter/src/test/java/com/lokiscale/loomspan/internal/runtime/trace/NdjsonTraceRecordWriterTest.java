package com.lokiscale.loomspan.internal.runtime.trace;

import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class NdjsonTraceRecordWriterTest {

    @Test
    void writesMonotonicSequenceAndSessionNamedTempFile() throws Exception {
        String sessionId = "trace-writer-session";
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                sessionId,
                "test.entry",
                TracePersistencePolicy.ALWAYS,
                Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC));

        handle.append(TraceRecordType.MODEL_REQUEST_SENT, java.util.Map.of("segment", "test"), java.util.Map.of("objective", "first"));
        handle.append(TraceRecordType.MODEL_REQUEST_SENT, java.util.Map.of("segment", "test"), java.util.Map.of("objective", "second"));

        List<TraceRecord> records = new ArrayList<>();
        handle.readRecords(records::add);

        assertThat(handle.tracePath().getFileName().toString())
                .contains(sessionId)
                .endsWith(".execution-trace.ndjson");
        assertThat(Files.exists(handle.tracePath())).isTrue();
        String ndjson = Files.readString(handle.tracePath());
        assertThat(ndjson).endsWith("\n").doesNotContain("\r\n");
        assertThat(ndjson.lines()).hasSize(4);
        assertThat(records)
                .extracting(TraceRecord::recordType)
                .contains(TraceRecordType.TRACE_STARTED, TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED, TraceRecordType.MODEL_REQUEST_SENT);
        assertThat(records.stream().mapToLong(TraceRecord::sequence).toArray())
                .isSorted();
    }
}
