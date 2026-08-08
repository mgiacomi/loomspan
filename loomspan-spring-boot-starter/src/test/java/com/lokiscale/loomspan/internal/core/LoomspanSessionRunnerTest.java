package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.runtime.trace.ExecutionTraceReaders;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.ObservationCompletionDisposition;
import com.lokiscale.loomspan.internal.runtime.observation.DefaultExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivity;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivityKind;
import com.lokiscale.loomspan.internal.runtime.trace.ImmediateCompletionRetention;

import org.junit.jupiter.api.Test;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.AuthorityUtils;

import java.nio.file.Files;
import java.nio.file.Path;
import java.io.IOException;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Consumer;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LoomspanSessionRunnerTest {

    @Test
    void defaultsNewSessionsToRetainTraceOnError() {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4);

        TracePersistencePolicy persistencePolicy = sessionRunner.callWithNewSession("test.entry",
                session -> session.getExecutionTrace().persistencePolicy());

        assertThat(persistencePolicy).isEqualTo(TracePersistencePolicy.ONERROR);
    }

    @Test
    void finalizesStandaloneRunnerSessionsAndWritesTerminalTraceRecord() throws Exception {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4, TracePersistencePolicy.ALWAYS);

        String tracePathText = sessionRunner.callWithNewSession("test.entry", session -> {
            appendRecord(session, TraceRecordType.MODEL_REQUEST_SENT, Instant.parse("2026-03-15T12:00:00Z"), Map.of("segment", "test"), Map.of("objective", "runner"));
            return session.getExecutionTrace().filePath();
        });

        var tracePath = java.nio.file.Path.of(tracePathText);
        try {
            assertThat(Files.exists(tracePath)).isTrue();
            List<TraceRecord> records = new ArrayList<>();
            ExecutionTraceReaders.ndjson().read(tracePath, records::add);
            assertThat(records).extracting(TraceRecord::recordType)
                    .contains(TraceRecordType.TRACE_COMPLETED);
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void retainsErroredStandaloneRunnerTracesUnderOnErrorPolicy() throws Exception {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4, TracePersistencePolicy.ONERROR);

        java.util.concurrent.atomic.AtomicReference<String> tracePathText = new java.util.concurrent.atomic.AtomicReference<>();
        String sessionId = null;
        try {
            sessionRunner.callWithNewSession("test.entry", session -> {
                appendRecord(session, TraceRecordType.MODEL_REQUEST_SENT, Instant.parse("2026-03-15T12:00:00Z"), Map.of("segment", "test"), Map.of("objective", "runner"));
                tracePathText.set(session.getExecutionTrace().filePath());
                throw new IllegalStateException(session.getSessionId());
            });
        }
        catch (IllegalStateException ex) {
            sessionId = ex.getMessage();
        }

        assertThat(sessionId).isNotBlank();
        assertThat(tracePathText.get()).isNotBlank();
        var tracePath = java.nio.file.Path.of(tracePathText.get());
        try {
            assertThat(Files.exists(tracePath)).isTrue();
            List<TraceRecord> records = new ArrayList<>();
            ExecutionTraceReaders.ndjson().read(tracePath, records::add);
            assertThat(records.getLast().recordType()).isEqualTo(TraceRecordType.TRACE_COMPLETED);
            assertThat(records.getLast().metadata()).containsEntry("errored", true);
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void recordsFailedTerminalStatusWhenStandaloneRunnerActionFailsBeforeOpeningFrames() throws Exception {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4, TracePersistencePolicy.ONERROR);

        java.util.concurrent.atomic.AtomicReference<String> tracePathText = new java.util.concurrent.atomic.AtomicReference<>();
        assertThatThrownBy(() -> sessionRunner.callWithNewSession("test.entry", session -> {
            tracePathText.set(session.getExecutionTrace().filePath());
            throw new IllegalArgumentException("boom");
        }))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessage("boom");

        assertThat(tracePathText.get()).isNotBlank();
        var tracePath = java.nio.file.Path.of(tracePathText.get());
        try {
            List<TraceRecord> records = new ArrayList<>();
            ExecutionTraceReaders.ndjson().read(tracePath, records::add);
            assertThat(records.getLast().recordType()).isEqualTo(TraceRecordType.TRACE_COMPLETED);
            assertThat(records.getLast().metadata())
                    .containsEntry("errored", true)
                    .containsEntry("outcome", "FAILED")
                    .containsKey("terminalFailureId")
                    .doesNotContainValue("boom");
            TraceRecord error = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                    .findFirst()
                    .orElseThrow();
            assertThat(error.metadata().get("failureId"))
                    .isEqualTo(records.getLast().metadata().get("terminalFailureId"));
            assertThat(error.data().path("exceptionType").asText()).isEqualTo(IllegalArgumentException.class.getName());
            assertThat(error.data().path("message").asText()).isEqualTo("Session execution failed");
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void createsDistinctSessionsAcrossConcurrentVirtualThreads() throws Exception {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4);

        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            Future<String> first = executor.submit(() ->
                    sessionRunner.callWithNewSession("test.entry", session -> LoomspanSession.getCurrentSession().getSessionId()));
            Future<String> second = executor.submit(() ->
                    sessionRunner.callWithNewSession("test.entry", session -> LoomspanSession.getCurrentSession().getSessionId()));

            assertThat(Set.of(first.get(), second.get())).hasSize(2);
        }
    }

    @Test
    void isolatesFrameMutationAcrossConcurrentVirtualThreads() throws InterruptedException, ExecutionException {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4);

        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            Future<String> first = executor.submit(() -> sessionRunner.callWithNewSession("test.entry", session -> {
                session.pushFrame(frame("frame-1", "route.one"));
                String result = session.getSessionId() + ":" + session.getFramesSnapshot().size() + ":" + session.peekFrame().route();
                session.popFrame();
                return result;
            }));
            Future<String> second = executor.submit(() -> sessionRunner.callWithNewSession("test.entry", session -> {
                session.pushFrame(frame("frame-2", "route.two"));
                String result = session.getSessionId() + ":" + session.getFramesSnapshot().size() + ":" + session.peekFrame().route();
                session.popFrame();
                return result;
            }));

            assertThat(first.get()).contains(":1:route.one");
            assertThat(second.get()).contains(":1:route.two");
            assertThat(first.get()).isNotEqualTo(second.get());
        }
    }

    @Test
    void rejectsStandaloneFinalizationWhenFramesRemainOpen() throws Exception {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4, TracePersistencePolicy.ALWAYS);
        java.util.concurrent.atomic.AtomicReference<LoomspanSession> sessionRef = new java.util.concurrent.atomic.AtomicReference<>();
        java.util.concurrent.atomic.AtomicReference<String> tracePathText = new java.util.concurrent.atomic.AtomicReference<>();

        assertThatThrownBy(() -> sessionRunner.callWithNewSession("test.entry", session -> {
            sessionRef.set(session);
            tracePathText.set(session.getExecutionTrace().filePath());
            session.pushFrame(frame("frame-1", "route.one"));
            return "unreachable";
        }))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Cannot finalize standalone session");

        LoomspanSession session = sessionRef.get();
        assertThat(session).isNotNull();
        assertThat(session.getExecutionTrace().completed()).isTrue();
        assertThat(session.getExecutionTrace().errored()).isTrue();

        var tracePath = java.nio.file.Path.of(tracePathText.get());
        try {
            assertThat(Files.exists(tracePath)).isTrue();
            List<TraceRecord> records = new ArrayList<>();
            ExecutionTraceReaders.ndjson().read(tracePath, records::add);
            assertThat(records.getLast().recordType()).isEqualTo(TraceRecordType.TRACE_COMPLETED);
            assertThat(records.getLast().metadata())
                    .containsEntry("errored", true)
                    .containsEntry("remainingFrames", 1)
                    .containsEntry("entryPoint", "session-runner");
        }
        finally {
            Files.deleteIfExists(tracePath);
        }
    }

    @Test
    void isolatesJournalMutationAcrossConcurrentVirtualThreads() throws InterruptedException, ExecutionException {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4);

        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            Future<String> first = executor.submit(() -> sessionRunner.callWithNewSession("test.entry", session -> {
                ExecutionPlan plan = plan("plan-first");
                appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", plan.planId()), plan);
                appendRecord(session, TraceRecordType.TOOL_CALL_REQUESTED, Instant.parse("2026-03-15T12:00:01Z"), Map.of(), Map.of("route", "tool.one"));
                return session.getSessionId()
                        + ":"
                        + session.getJournalSnapshot().size()
                        + ":"
                        + session.getJournalSnapshot().get(0).type().name();
            }));
            Future<String> second = executor.submit(() -> sessionRunner.callWithNewSession("test.entry", session -> {
                ExecutionPlan plan = plan("plan-second");
                appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:02Z"), Map.of("planId", plan.planId()), plan);
                session.markTraceErrored();
                appendRecord(session, TraceRecordType.ERROR_RECORDED, Instant.parse("2026-03-15T12:00:03Z"), Map.of(), Map.of("message", "boom"));
                return session.getSessionId()
                        + ":"
                        + session.getJournalSnapshot().size()
                        + ":"
                        + session.getJournalSnapshot().get(0).type().name();
            }));

            assertThat(first.get()).contains(":2:PLAN_CREATED");
            assertThat(second.get()).contains(":2:PLAN_CREATED");
            assertThat(first.get()).isNotEqualTo(second.get());
        }
    }

    @Test
    void seedsAuthenticationIntoNewSessionWhenProvided() {
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4);
        var authentication = UsernamePasswordAuthenticationToken.authenticated(
                "user",
                "pw",
                AuthorityUtils.createAuthorityList("ROLE_ALLOWED"));

        String authority = sessionRunner.callWithNewSession("test.entry", authentication, session ->
                session.getAuthentication()
                        .orElseThrow()
                        .getAuthorities()
                        .iterator()
                        .next()
                        .getAuthority());

        assertThat(authority).isEqualTo("ROLE_ALLOWED");
    }

    @Test
    void usesConfiguredClockForLiveTraceTimestamps() {
        Clock fixedClock = Clock.fixed(Instant.parse("2026-03-15T12:34:56Z"), ZoneOffset.UTC);
        LoomspanSessionRunner sessionRunner = new LoomspanSessionRunner(4, TracePersistencePolicy.ALWAYS, fixedClock);

        Instant timestamp = sessionRunner.callWithNewSession("test.entry", session -> {
            session.appendTraceRecord(TraceRecordType.MODEL_REQUEST_SENT, Map.of("segment", "test"), Map.of("objective", "runner"));
            List<TraceRecord> records = new ArrayList<>();
            session.readTraceRecords(records::add);
            return records.getLast().timestamp();
        });

        assertThat(timestamp).isEqualTo(Instant.parse("2026-03-15T12:34:56Z"));
    }

    @Test
    void snapshotsConfiguredLimitsWhenTheTraceIsCreated() {
        LoomspanProperties.Session.Quotas quotas = new LoomspanProperties.Session.Quotas();
        quotas.setMaxSkillInvocations(7);
        quotas.setMaxToolInvocations(11);
        quotas.setMaxLinterRetries(3);
        quotas.setMaxModelCalls(5);
        quotas.setMaxUsageUnits(1234);
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.ALWAYS, Clock.systemUTC(),
                (sessionId, entrySkill) -> com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandle.INSTANCE,
                ImmediateCompletionRetention.INSTANCE, quotas);

        @SuppressWarnings("unchecked")
        Map<String, Integer> snapshot = runner.callWithNewSession("test.entry", session -> {
            quotas.setMaxModelCalls(99);
            List<TraceRecord> records = new ArrayList<>();
            session.readTraceRecords(records::add);
            return (Map<String, Integer>) records.getFirst().metadata().get("configuredLimits");
        });

        assertThat(snapshot).containsExactlyInAnyOrderEntriesOf(Map.of(
                "maxSkillInvocations", 7,
                "maxToolInvocations", 11,
                "maxLinterRetries", 3,
                "maxModelCalls", 5,
                "maxUsageUnits", 1234));
    }

    @Test
    void attachesObservationBeforeTraceInitialization() {
        RecordingObservationFactory observation = new RecordingObservationFactory();
        Clock clock = Clock.fixed(Instant.parse("2026-03-15T12:34:56Z"), ZoneOffset.UTC);
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.NEVER, clock, observation);

        runner.callWithNewSession("test.entry", session -> {
            assertThat(observation.handles).hasSize(1);
            assertThat(observation.handles.getFirst().records)
                    .extracting(TraceRecord::recordType)
                    .startsWith(
                            TraceRecordType.TRACE_STARTED,
                            TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED);
            return "ok";
        });

        RecordingObservationHandle handle = observation.handles.getFirst();
        assertThat(handle.records).extracting(TraceRecord::sequence).startsWith(1L, 2L);
        assertThat(handle.dispositions).singleElement()
                .extracting(ObservationCompletionDisposition::status)
                .isEqualTo(ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED);
    }

    @Test
    void optionalObservationFailureDoesNotChangeSuccessfulResult() {
        ExecutionObservationHandleFactory throwingFactory = (sessionId, entrySkill) -> new ExecutionObservationHandle() {
            @Override
            public void recordAppended(TraceRecord record) {
                throw new IllegalStateException("optional");
            }

            @Override
            public void close(ObservationCompletionDisposition disposition) {
                throw new IllegalStateException("optional");
            }
        };
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.NEVER, Clock.systemUTC(), throwingFactory);

        String result = runner.callWithNewSession("test.entry", session -> "unchanged");
        assertThat(result).isEqualTo("unchanged");
    }

    @Test
    void usesCoreFailureDispositionWhenJournalProjectionFails() throws Exception {
        DefaultExecutionObservationHandleFactory observation =
                new DefaultExecutionObservationHandleFactory();
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.ALWAYS, Clock.systemUTC(), observation);
        AtomicReference<java.nio.file.Path> tracePath = new AtomicReference<>();

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> {
            java.nio.file.Path path = java.nio.file.Path.of(session.getExecutionTrace().filePath());
            tracePath.set(path);
            try {
                Files.writeString(path, "not-json\n", java.nio.file.StandardOpenOption.TRUNCATE_EXISTING);
            }
            catch (java.io.IOException ex) {
                throw new IllegalStateException(ex);
            }
            return "unreachable";
        }))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Failed to finalize execution trace");

        try {
            assertThat(observation.registry().activeCount()).isZero();
            assertThat(observation.replayBuffer().replayAfter(0, 20).activities())
                    .extracting(ExecutionActivity::kind)
                    .contains(ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED)
                    .doesNotContain(ExecutionActivityKind.TRACE_COMPLETED);
        }
        finally {
            Files.deleteIfExists(tracePath.get());
        }
    }

    @Test
    void closesHandleWhenTraceConstructionFailsAfterRegistration() {
        DefaultExecutionObservationHandleFactory observation =
                new DefaultExecutionObservationHandleFactory();
        Clock clock = Clock.fixed(Instant.parse("2026-07-24T12:00:00Z"), ZoneOffset.UTC);
        InternalExecutionTraceHandleFactory failingFactory = (sessionId, entrySkill, policy, ignoredClock, handle) -> {
            handle.recordAppended(new TraceRecord(
                    "trace-construction", sessionId, 1L, clock.instant(),
                    TraceRecordType.TRACE_STARTED, null, null, null, null,
                    "thread", Map.of(), null));
            throw new IllegalStateException("trace construction failed");
        };
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.ALWAYS, clock, observation, failingFactory);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> "unreachable"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("trace construction failed");

        assertThat(observation.registry().activeCount()).isZero();
        assertThat(observation.replayBuffer().replayAfter(0, 10).activities())
                .extracting(ExecutionActivity::kind)
                .containsExactly(
                        ExecutionActivityKind.TRACE_STARTED,
                        ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED);
    }

    @Test
    void usesCoreFailureDispositionWhenCompletionAppendFails() throws Exception {
        assertInjectedFinalizationFailure(false);
    }

    @Test
    void usesCoreFailureDispositionWhenRetentionDeletionFailsAfterCompletedFlag() throws Exception {
        assertInjectedFinalizationFailure(true);
    }

    @Test
    void removesActiveObservationWhenFinalizationThrowsUncheckedFailure() throws Exception {
        DefaultExecutionObservationHandleFactory observation =
                new DefaultExecutionObservationHandleFactory();
        AtomicReference<Path> tracePath = new AtomicReference<>();
        SecurityException failure = new SecurityException("retention access denied");
        InternalExecutionTraceHandleFactory traceFactory = (sessionId, entrySkill, policy, clock, handle) -> {
            com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle delegate =
                    new com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle(
                            sessionId, entrySkill, TracePersistencePolicy.ALWAYS, clock, handle);
            tracePath.set(delegate.tracePath());
            return new FailingFinalizationTraceHandle(delegate, failure);
        };
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.ALWAYS, Clock.systemUTC(), observation, traceFactory);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> "result"))
                .isSameAs(failure);

        try {
            assertThat(observation.registry().activeCount()).isZero();
            assertThat(observation.replayBuffer().replayAfter(0, 20).activities())
                    .extracting(ExecutionActivity::kind)
                    .contains(ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED)
                    .doesNotContain(ExecutionActivityKind.TRACE_COMPLETED);
        }
        finally {
            Files.deleteIfExists(tracePath.get());
        }
    }

    private void assertInjectedFinalizationFailure(boolean failAfterCompletion) throws Exception {
        DefaultExecutionObservationHandleFactory observation =
                new DefaultExecutionObservationHandleFactory();
        AtomicReference<Path> tracePath = new AtomicReference<>();
        InternalExecutionTraceHandleFactory traceFactory = (sessionId, entrySkill, policy, clock, handle) -> {
            com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle delegate =
                    new com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle(
                            sessionId, entrySkill, TracePersistencePolicy.ALWAYS, clock, handle);
            tracePath.set(delegate.tracePath());
            return new FailingFinalizationTraceHandle(delegate, failAfterCompletion);
        };
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.ALWAYS, Clock.systemUTC(), observation, traceFactory);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> "result"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Failed to finalize execution trace");

        try {
            assertThat(observation.registry().activeCount()).isZero();
            assertThat(observation.replayBuffer().replayAfter(0, 20).activities())
                    .extracting(ExecutionActivity::kind)
                    .contains(ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED)
                    .doesNotContain(ExecutionActivityKind.TRACE_COMPLETED);
        }
        finally {
            Files.deleteIfExists(tracePath.get());
        }
    }

    private static final class RecordingObservationFactory implements ExecutionObservationHandleFactory {
        private final List<RecordingObservationHandle> handles = new CopyOnWriteArrayList<>();

        @Override
        public ExecutionObservationHandle create(String sessionId, String entrySkill) {
            RecordingObservationHandle handle = new RecordingObservationHandle();
            handles.add(handle);
            return handle;
        }
    }

    private static final class FailingFinalizationTraceHandle implements ExecutionTraceHandle {
        private final ExecutionTraceHandle delegate;
        private final boolean failAfterCompletion;
        private final RuntimeException uncheckedFailure;

        private FailingFinalizationTraceHandle(
                ExecutionTraceHandle delegate,
                boolean failAfterCompletion) {
            this.delegate = delegate;
            this.failAfterCompletion = failAfterCompletion;
            this.uncheckedFailure = null;
        }

        private FailingFinalizationTraceHandle(
                ExecutionTraceHandle delegate,
                RuntimeException uncheckedFailure) {
            this.delegate = delegate;
            this.failAfterCompletion = true;
            this.uncheckedFailure = uncheckedFailure;
        }

        @Override
        public TraceRecord append(
                TraceRecordType recordType,
                ExecutionFrame frame,
                TraceFrameType frameType,
                Map<String, Object> metadata,
                Object data) throws IOException {
            return delegate.append(recordType, frame, frameType, metadata, data);
        }

        @Override
        public TraceRecord append(
                TraceRecordType recordType,
                Map<String, Object> metadata,
                Object data) throws IOException {
            return delegate.append(recordType, metadata, data);
        }

        @Override
        public ExecutionTrace snapshot() {
            return delegate.snapshot();
        }

        @Override
        public Path tracePath() {
            return delegate.tracePath();
        }

        @Override
        public void markErrored() {
            delegate.markErrored();
        }

        @Override
        public java.util.Optional<FinalizedTraceArtifact> finalizeTrace(TraceCompletion completion) throws IOException {
            if (failAfterCompletion) {
                delegate.finalizeTrace(completion);
            }
            if (uncheckedFailure != null) {
                throw uncheckedFailure;
            }
            throw new IOException(failAfterCompletion
                    ? "retention deletion failed"
                    : "completion append failed");
        }

        @Override
        public void readRecords(Consumer<TraceRecord> consumer) throws IOException {
            delegate.readRecords(consumer);
        }
    }

    private static final class RecordingObservationHandle implements ExecutionObservationHandle {
        private final List<TraceRecord> records = new CopyOnWriteArrayList<>();
        private final List<ObservationCompletionDisposition> dispositions = new CopyOnWriteArrayList<>();

        @Override
        public void recordAppended(TraceRecord record) {
            records.add(record);
        }

        @Override
        public void close(ObservationCompletionDisposition disposition) {
            dispositions.add(disposition);
        }
    }

    private static ExecutionFrame frame(String frameId, String route) {
        return new ExecutionFrame(
                frameId,
                null,
                OperationType.CAPABILITY,
                TraceFrameType.ROOT_MISSION,
                route,
                Map.of("route", route),
                Instant.parse("2026-03-15T12:00:00Z"));
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

    private static ExecutionPlan plan(String planId) {
        return new ExecutionPlan(
                planId,
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of());
    }
}
