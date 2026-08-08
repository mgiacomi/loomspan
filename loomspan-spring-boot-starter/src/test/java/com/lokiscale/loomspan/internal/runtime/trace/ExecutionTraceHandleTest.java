package com.lokiscale.loomspan.internal.runtime.trace;

import com.lokiscale.loomspan.internal.core.TraceCompletion;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import com.lokiscale.loomspan.internal.runtime.observation.ObservationCompletionDisposition;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ExecutionTraceHandleTest {

    @TempDir
    Path tempDir;

    @Test
    void appliesNeverOnErrorAndAlwaysPersistencePolicies() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);

        DefaultExecutionTraceHandle never = new DefaultExecutionTraceHandle("never-trace", "test.entry", TracePersistencePolicy.NEVER, clock);
        never.finalizeTrace(completion(TraceOutcome.SUCCEEDED));
        assertThat(Files.exists(never.tracePath())).isFalse();

        DefaultExecutionTraceHandle onError = new DefaultExecutionTraceHandle("onerror-trace", "test.entry", TracePersistencePolicy.ONERROR, clock);
        onError.markErrored();
        onError.append(TraceRecordType.ERROR_RECORDED, java.util.Map.of("kind", "runtime"), java.util.Map.of("message", "boom"));
        onError.finalizeTrace(completion(TraceOutcome.FAILED));
        assertThat(Files.exists(onError.tracePath())).isTrue();

        DefaultExecutionTraceHandle always = new DefaultExecutionTraceHandle("always-trace", "test.entry", TracePersistencePolicy.ALWAYS, clock);
        always.finalizeTrace(completion(TraceOutcome.SUCCEEDED));
        assertThat(Files.exists(always.tracePath())).isTrue();
    }

    @Test
    void honorsExplicitTimestampOverridesInTraceEnvelope() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle("override-trace", "test.entry", TracePersistencePolicy.ALWAYS, clock);

        handle.append(
                TraceRecordType.MODEL_REQUEST_SENT,
                java.util.Map.of("timestampOverride", "2026-03-20T05:06:07Z"),
                java.util.Map.of("objective", "after"));

        List<TraceRecord> records = readRecords(handle);

        assertThat(records.getLast().recordType()).isEqualTo(TraceRecordType.MODEL_REQUEST_SENT);
        assertThat(records.getLast().timestamp()).isEqualTo(Instant.parse("2026-03-20T05:06:07Z"));
        Files.deleteIfExists(handle.tracePath());
    }

    @Test
    void usesSessionNamedTempFiles() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle("shared-session", "test.entry", TracePersistencePolicy.ALWAYS, clock);

        assertThat(handle.tracePath().getFileName().toString())
                .startsWith("shared-session.")
                .endsWith(".execution-trace.ndjson");

        Files.deleteIfExists(handle.tracePath());
    }

    @Test
    void usesDistinctTraceFilesForRepeatedRunsOfTheSameSessionId() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        DefaultExecutionTraceHandle first = new DefaultExecutionTraceHandle("shared-session", "test.entry", TracePersistencePolicy.ALWAYS, clock);
        DefaultExecutionTraceHandle second = new DefaultExecutionTraceHandle("shared-session", "test.entry", TracePersistencePolicy.ALWAYS, clock);

        assertThat(first.tracePath()).isNotEqualTo(second.tracePath());
        assertThat(first.tracePath().getFileName().toString()).startsWith("shared-session.");
        assertThat(second.tracePath().getFileName().toString()).startsWith("shared-session.");

        Files.deleteIfExists(first.tracePath());
        Files.deleteIfExists(second.tracePath());
    }

    @Test
    void rejectsAppendsAfterTraceFinalization() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle("completed-trace", "test.entry", TracePersistencePolicy.ALWAYS, clock);

        handle.finalizeTrace(completion(TraceOutcome.SUCCEEDED));

        assertThatThrownBy(() -> handle.append(TraceRecordType.MODEL_REQUEST_SENT, java.util.Map.of(), java.util.Map.of("objective", "late")))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("already completed");

        Files.deleteIfExists(handle.tracePath());
    }

    @Test
    void constructsDescriptorFromSizeCapturedBeforeAsynchronousDeletion() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        CompletionGraceRetention deletionDuringRetention = new CompletionGraceRetention() {
            @Override
            public java.util.Optional<RetainedArtifact> retainOrDelete(
                    Path artifactPath,
                    Instant finalizedAt,
                    String traceId,
                    String sessionId) throws IOException {
                long sizeBytes = Files.size(artifactPath);
                Files.delete(artifactPath);
                return java.util.Optional.of(new RetainedArtifact(
                        finalizedAt.plusSeconds(1),
                        sizeBytes));
            }

            @Override
            public java.util.Optional<ArtifactLease> acquire(
                    com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact artifact)
            {
                return java.util.Optional.empty();
            }

            @Override
            public void close() {
            }
        };
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                "short-grace-trace",
                "test.entry",
                TracePersistencePolicy.NEVER,
                clock,
                com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandle.INSTANCE,
                deletionDuringRetention);

        var artifact = handle.finalizeTrace(completion(TraceOutcome.SUCCEEDED)).orElseThrow();

        assertThat(artifact.sizeBytes()).isPositive();
        assertThat(handle.tracePath()).doesNotExist();
    }

    @Test
    void publishesTraceStartedAndCapturePolicyExactlyOnce() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        RecordingObservationHandle observation = new RecordingObservationHandle();
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                "observed-trace", "test.entry", TracePersistencePolicy.ALWAYS, clock, observation);

        assertThat(observation.records)
                .extracting(TraceRecord::recordType)
                .containsExactly(
                        TraceRecordType.TRACE_STARTED,
                        TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED);
        assertThat(observation.records).extracting(TraceRecord::sequence).containsExactly(1L, 2L);
        assertThat(observation.records).extracting(TraceRecord::traceId).containsOnly(handle.snapshot().traceId());
        Files.deleteIfExists(handle.tracePath());
    }

    @Test
    void publishesCompleteLogicalChunkedRecordOnlyAfterAllChunksSucceed() throws Exception {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        RecordingObservationHandle observation = new RecordingObservationHandle();
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                "chunked-trace", "test.entry", TracePersistencePolicy.ALWAYS, clock, observation);
        String payload = "x".repeat(4_097);

        TraceRecord persistedEnvelope = handle.append(
                TraceRecordType.MODEL_RESPONSE_RECEIVED,
                java.util.Map.of("attemptNumber", 1),
                payload);

        assertThat(persistedEnvelope.data()).isNull();
        assertThat(observation.records.getLast().recordType())
                .isEqualTo(TraceRecordType.MODEL_RESPONSE_RECEIVED);
        assertThat(observation.records.getLast().data().asText()).isEqualTo(payload);
        assertThat(observation.records)
                .noneMatch(record -> record.recordType() == TraceRecordType.PAYLOAD_CHUNK_APPENDED);

        List<TraceRecord> physical = readRecords(handle);
        assertThat(physical)
                .extracting(TraceRecord::recordType)
                .contains(TraceRecordType.PAYLOAD_CHUNK_APPENDED);
        Files.deleteIfExists(handle.tracePath());
    }

    @Test
    void doesNotPublishWhenEnvelopeWriteFails() {
        assertChunkWriteFailurePublishesNothing(3, 9_000);
    }

    @Test
    void doesNotPublishWhenMiddleChunkWriteFails() {
        assertChunkWriteFailurePublishesNothing(5, 9_000);
    }

    @Test
    void doesNotPublishWhenFinalChunkWriteFails() {
        assertChunkWriteFailurePublishesNothing(6, 9_000);
    }

    private void assertChunkWriteFailurePublishesNothing(int failingWrite, int payloadLength) {
        Clock clock = Clock.fixed(Instant.parse("2026-03-24T12:00:00Z"), ZoneOffset.UTC);
        RecordingObservationHandle observation = new RecordingObservationHandle();
        Path tracePath = tempDir.resolve("failed-" + failingWrite + ".ndjson");
        ControllableWriter writer = new ControllableWriter(tracePath, failingWrite);
        DefaultExecutionTraceHandle handle = new DefaultExecutionTraceHandle(
                "trace-" + failingWrite,
                "session-" + failingWrite,
                "test.entry",
                tracePath,
                TracePersistencePolicy.ALWAYS,
                clock,
                () -> "payload",
                "thread",
                "trace.ndjson",
                observation,
                writer);

        assertThatThrownBy(() -> handle.append(
                TraceRecordType.MODEL_RESPONSE_RECEIVED,
                java.util.Map.of(),
                "x".repeat(payloadLength)))
                .isInstanceOf(IOException.class);
        assertThat(observation.records)
                .extracting(TraceRecord::recordType)
                .containsExactly(
                        TraceRecordType.TRACE_STARTED,
                        TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED);
    }

    private static final class ControllableWriter implements TraceRecordWriter {
        private final NdjsonTraceRecordWriter delegate;
        private final int failingWrite;
        private int writes;

        private ControllableWriter(Path tracePath, int failingWrite) {
            this.delegate = new NdjsonTraceRecordWriter(tracePath);
            this.failingWrite = failingWrite;
        }

        @Override
        public void append(TraceRecord record) throws IOException {
            writes++;
            if (writes == failingWrite) {
                throw new IOException("selected write failure");
            }
            delegate.append(record);
        }
    }

    private static final class RecordingObservationHandle implements ExecutionObservationHandle {
        private final List<TraceRecord> records = new ArrayList<>();

        @Override
        public void recordAppended(TraceRecord record) {
            records.add(record);
        }

        @Override
        public void close(ObservationCompletionDisposition disposition) {
        }
    }

    private static List<TraceRecord> readRecords(DefaultExecutionTraceHandle handle) throws Exception {
        List<TraceRecord> records = new ArrayList<>();
        handle.readRecords(records::add);
        return records;
    }

    private static TraceCompletion completion(TraceOutcome outcome) {
        return new TraceCompletion(
                outcome,
                SessionUsageSnapshot.empty(),
                outcome == TraceOutcome.SUCCEEDED ? null : "failure",
                java.util.Map.of("status", outcome == TraceOutcome.SUCCEEDED ? "ok" : "error"));
    }
}
