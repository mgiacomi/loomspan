package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.authority.AuthorityUtils;

import java.io.IOException;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.junit.jupiter.api.Assertions.assertTimeoutPreemptively;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.any;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.eq;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class LoomspanSessionTest {

    @Test
    void rejectsNullTraceMetadataValues() {
        LoomspanSession session = TestLoomspanSessions.withId("null-trace-metadata", "test.entry", 3);
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("capabilityName", "lookupCustomer");
        metadata.put("linkedTaskId", null);

        assertThatThrownBy(() -> session.appendTraceRecord(TraceRecordType.TOOL_CALL_STARTED, metadata, Map.of()))
                .isInstanceOf(NullPointerException.class)
                .hasMessageContaining("metadata value");
    }

    @Test
    void journalProjectionFailureUsesCanonicalDiagnosticRecorder() throws Exception {
        ExecutionTraceHandle handle = failingProjectionHandle();
        AtomicReference<TraceCompletion> finalized = new AtomicReference<>();
        doAnswer(invocation -> {
            finalized.set(invocation.getArgument(0));
            return Optional.empty();
        }).when(handle).finalizeTrace(any());
        LoomspanSession session = TestLoomspanSessions.withTraceHandle(
                "projection-session", "test.entry", Clock.systemUTC(), handle, () -> "projection-failure");

        assertThatThrownBy(() -> session.finalizeTrace(new TraceCompletion(
                TraceOutcome.SUCCEEDED,
                com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty(),
                null,
                Map.of())))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Failed to finalize execution trace");

        ArgumentCaptor<Map<String, Object>> metadata = ArgumentCaptor.forClass(Map.class);
        ArgumentCaptor<Object> data = ArgumentCaptor.forClass(Object.class);
        verify(handle).append(eq(TraceRecordType.ERROR_RECORDED), metadata.capture(), data.capture());
        assertThat(metadata.getValue()).containsEntry("failureId", "projection-failure");
        assertThat(((Map<?, ?>) data.getValue()).get("diagnostics")).isNotNull();
        assertThat(finalized.get().outcome()).isEqualTo(TraceOutcome.FAILED);
        assertThat(finalized.get().terminalFailureId()).isEqualTo("projection-failure");
    }

    @Test
    void journalProjectionFailureDoesNotDuplicateAnExistingTerminalFailure() throws Exception {
        ExecutionTraceHandle handle = failingProjectionHandle();
        AtomicReference<TraceCompletion> finalized = new AtomicReference<>();
        doAnswer(invocation -> {
            finalized.set(invocation.getArgument(0));
            return Optional.empty();
        }).when(handle).finalizeTrace(any());
        LoomspanSession session = TestLoomspanSessions.withTraceHandle(
                "projection-existing-session", "test.entry", Clock.systemUTC(), handle, () -> "original-failure");
        String failureId = session.recordFailure(new IllegalStateException("original"), Map.of("message", "failed"));

        assertThatThrownBy(() -> session.finalizeTrace(new TraceCompletion(
                TraceOutcome.FAILED,
                com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty(),
                failureId,
                Map.of())))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Failed to finalize execution trace");

        verify(handle, times(1)).append(eq(TraceRecordType.ERROR_RECORDED), any(), any());
        assertThat(finalized.get().terminalFailureId()).isEqualTo(failureId);
    }

    private static ExecutionTraceHandle failingProjectionHandle() throws IOException {
        ExecutionTraceHandle handle = mock(ExecutionTraceHandle.class);
        when(handle.snapshot()).thenReturn(new ExecutionTrace(
                "projection-trace", "projection-session", null, TracePersistencePolicy.ALWAYS, false, false));
        doThrow(new IOException("projection read failed")).when(handle).readRecords(any());
        return handle;
    }

    @Test
    void recordsThrowableOnceAndReusesRecordedCause() {
        LoomspanSession session = new LoomspanSession(8, "test.skill");
        IllegalStateException cause = new IllegalStateException("same");
        String first = session.recordFailure(cause, Map.of("message", "failed"));
        String repeated = session.recordFailure(cause, Map.of("message", "failed"));
        String wrapped = session.recordFailure(new RuntimeException("wrapper", cause), Map.of("message", "failed"));
        String distinct = session.recordFailure(new IllegalStateException("same"), Map.of("message", "failed"));
        assertThat(repeated).isEqualTo(first);
        assertThat(wrapped).isEqualTo(first);
        assertThat(distinct).isNotEqualTo(first);
    }

    @Test
    void linksTheCanonicalTerminalErrorToTheRegisteredFinalProviderAttempt() {
        LoomspanSession session = new LoomspanSession(8, "test.skill");
        IllegalStateException cause = new IllegalStateException("provider failed");
        RuntimeException wrapper = new RuntimeException("call failed", cause);
        session.registerProviderFailure(cause, Map.of(
                "attemptId", "attempt-final", "retrySequenceId", "retry-sequence"));

        session.recordFailure(wrapper, Map.of("message", "failed"));

        java.util.List<TraceRecord> records = new java.util.ArrayList<>();
        session.readTraceRecords(records::add);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                .singleElement().satisfies(record -> assertThat(record.metadata())
                        .containsEntry("attemptId", "attempt-final")
                        .containsEntry("retrySequenceId", "retry-sequence"));
    }

    @Test
    void boundsCyclicAndDeepCauseTraversal() {
        LoomspanSession session = new LoomspanSession(8, "test.skill");
        RuntimeException first = new RuntimeException("first");
        RuntimeException second = new RuntimeException("second");
        first.initCause(second);
        second.initCause(first);

        assertTimeoutPreemptively(Duration.ofSeconds(1), () -> {
            String cycleId = session.recordFailure(first, Map.of("message", "failed"));
            assertThat(session.recordFailure(second, Map.of("message", "failed"))).isEqualTo(cycleId);

            Throwable deep = new IllegalStateException("root");
            for (int index = 0; index < 100; index++) {
                deep = new RuntimeException("wrapper-" + index, deep);
            }
            assertThat(session.recordFailure(deep, Map.of("message", "failed"))).isNotBlank();
        });
    }

    @Test
    void concurrentObservationsAppendOnceAndSessionsRemainIsolated() throws Exception {
        LoomspanSession session = new LoomspanSession(8, "test.skill");
        IllegalStateException failure = new IllegalStateException("shared");
        CountDownLatch start = new CountDownLatch(1);
        Set<String> ids = ConcurrentHashMap.newKeySet();
        try (var executor = Executors.newFixedThreadPool(8)) {
            var futures = java.util.stream.IntStream.range(0, 32)
                    .mapToObj(index -> executor.submit(() -> {
                        start.await();
                        ids.add(session.recordFailure(failure, Map.of("message", "failed")));
                        return null;
                    }))
                    .toList();
            start.countDown();
            for (var future : futures) {
                future.get();
            }
        }
        AtomicInteger errorRecords = new AtomicInteger();
        session.readTraceRecords(record -> {
            if (record.recordType() == TraceRecordType.ERROR_RECORDED) {
                errorRecords.incrementAndGet();
            }
        });
        assertThat(ids).hasSize(1);
        assertThat(errorRecords).hasValue(1);

        LoomspanSession other = new LoomspanSession(8, "test.skill");
        assertThat(other.recordFailure(failure, Map.of("message", "failed"))).isNotIn(ids);
    }

    @Test
    void normalizesEntrySkillOnceBeforeSupplyingTheSameValueToBothFactories() {
        String overBound = "😀".repeat(EntrySkillIdentity.MAX_CODE_POINTS + 1);
        AtomicReference<String> observationEntry = new AtomicReference<>();
        AtomicReference<String> traceEntry = new AtomicReference<>();
        ExecutionObservationHandle observation = mock(ExecutionObservationHandle.class);

        LoomspanSession session = new LoomspanSession(
                "session-normalized", overBound, 3, null, TracePersistencePolicy.ALWAYS, Clock.systemUTC(),
                (sessionId, entrySkill) -> {
                    observationEntry.set(entrySkill);
                    return observation;
                },
                (sessionId, entrySkill, policy, clock, handle) -> {
                    traceEntry.set(entrySkill);
                    return mock(ExecutionTraceHandle.class);
                });

        assertThat(session.entrySkill().codePointCount(0, session.entrySkill().length()))
                .isEqualTo(EntrySkillIdentity.MAX_CODE_POINTS);
        assertThat(observationEntry.get()).isSameAs(session.entrySkill());
        assertThat(traceEntry.get()).isSameAs(session.entrySkill());
    }

    @Test
    void rejectsBlankEntrySkillBeforeInvokingEitherFactory() {
        AtomicInteger factoryCalls = new AtomicInteger();

        assertThatThrownBy(() -> new LoomspanSession(
                "session-invalid", " \t", 3, null, TracePersistencePolicy.ALWAYS, Clock.systemUTC(),
                (sessionId, entrySkill) -> {
                    factoryCalls.incrementAndGet();
                    return mock(ExecutionObservationHandle.class);
                },
                (sessionId, entrySkill, policy, clock, handle) -> {
                    factoryCalls.incrementAndGet();
                    return mock(ExecutionTraceHandle.class);
                }))
                .isInstanceOf(IllegalArgumentException.class);

        assertThat(factoryCalls).hasValue(0);
    }

    @Test
    void createsSessionWithGeneratedIdAndConfiguredMaxDepth() {
        LoomspanSession session = new LoomspanSession(3, "test.entry");

        assertThat(session.getSessionId()).isNotBlank();
        assertThat(session.getMaxDepth()).isEqualTo(3);
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void pushAndPopFrameUsesLifoOrder() {
        LoomspanSession session = new LoomspanSession(3, "test.entry");
        ExecutionFrame first = frame("frame-1", "route.one");
        ExecutionFrame second = frame("frame-2", "route.two");

        session.pushFrame(first);
        session.pushFrame(second);

        assertThat(session.peekFrame()).isEqualTo(second);
        assertThat(session.getFramesSnapshot()).containsExactly(second, first);
        assertThat(session.popFrame()).isEqualTo(second);
        assertThat(session.popFrame()).isEqualTo(first);
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    @Test
    void throwsWhenPushingBeyondMaxDepthWithoutMutatingStack() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 1);
        ExecutionFrame first = frame("frame-1", "route.one");
        ExecutionFrame second = frame("frame-2", "route.two");
        session.pushFrame(first);

        assertThatThrownBy(() -> session.pushFrame(second))
                .isInstanceOf(LoomspanStackOverflowException.class)
                .hasMessageContaining("session-1")
                .hasMessageContaining("route.two")
                .hasMessageContaining("1");

        assertThat(session.peekFrame()).isEqualTo(first);
        assertThat(session.getFramesSnapshot()).containsExactly(first);
    }

    @Test
    void throwsWhenPoppingEmptyStack() {
        LoomspanSession session = new LoomspanSession(2, "test.entry");

        assertThatThrownBy(session::popFrame)
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("Cannot pop execution frame from an empty session stack.");
    }

    @Test
    void exposesImmutableFrameSnapshots() {
        LoomspanSession session = new LoomspanSession(2, "test.entry");
        session.pushFrame(frame("frame-1", "route.one"));

        List<ExecutionFrame> snapshot = session.getFramesSnapshot();

        assertThatThrownBy(() -> snapshot.add(frame("frame-2", "route.two")))
                .isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void appendsJournalEntriesInSequentialOrder() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 4);

        ExecutionPlan plan = plan("plan-1");
        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", plan.planId()), plan);
        appendRecord(
                session,
                TraceRecordType.TOOL_CALL_STARTED,
                Instant.parse("2026-03-15T12:00:01Z"),
                Map.of(),
                Map.of("route", "tool.run", "arguments", Map.of("id", 42)));
        appendError(session, Instant.parse("2026-03-15T12:00:02Z"), "boom");

        assertThat(session.getJournalSnapshot())
                .extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.PLAN_CREATED, JournalEntryType.TOOL_CALL, JournalEntryType.ERROR);
        assertThat(session.getJournalSnapshot())
                .extracting(JournalEntry::level)
                .containsExactly(JournalLevel.INFO, JournalLevel.INFO, JournalLevel.ERROR);
        assertThat(session.getJournalSnapshot().get(0).payload().get("planId").textValue()).isEqualTo("plan-1");
        assertThat(session.getJournalSnapshot().get(1).payload().get("capabilityName").textValue()).isEqualTo("tool.run");
        assertThat(session.getJournalSnapshot().get(1).payload().get("details").get("arguments").get("id").intValue()).isEqualTo(42);
        assertThat(session.getJournalSnapshot().get(2).payload().textValue()).isEqualTo("boom");
    }

    @Test
    void sessionBindsFramesToJournalEntries() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 4);
        ExecutionFrame frame = frame("frame-1", "route.one");

        session.pushFrame(frame);
        ExecutionPlan plan = plan("plan-1");
        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", plan.planId()), plan);
        appendRecord(
                session,
                TraceRecordType.TOOL_CALL_STARTED,
                Instant.parse("2026-03-15T12:00:01Z"),
                Map.of("capabilityName", "tool.run", "linkedTaskId", "task-1"),
                TaskExecutionEvent.linked("tool.run", "task-1", Map.of("arguments", Map.of("id", 42)), null));
        session.popFrame();

        assertThat(session.getJournalSnapshot()).allSatisfy(entry -> {
            assertThat(entry.frameId()).isEqualTo("frame-1");
            assertThat(entry.route()).isEqualTo("route.one");
        });
    }

    @Test
    void exposesImmutableJournalSnapshots() {
        LoomspanSession session = new LoomspanSession(2, "test.entry");
        ExecutionPlan plan = plan("plan-1");
        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", plan.planId()), plan);

        List<JournalEntry> snapshot = session.getJournalSnapshot();

        assertThatThrownBy(() -> snapshot.add(new JournalEntry(
                Instant.parse("2026-03-15T12:00:01Z"),
                JournalLevel.INFO,
                JournalEntryType.THOUGHT,
                new com.fasterxml.jackson.databind.ObjectMapper().valueToTree("extra"),
                null,
                null)))
                .isInstanceOf(UnsupportedOperationException.class);
    }

    @Test
    void journalsPlanCreationAndUpdateSeparatelyFromActivePlanState() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        ExecutionPlan created = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null)));
        ExecutionPlan updated = created.updateTask("task-1",
                task -> task.withStatus(PlanTaskStatus.COMPLETED, "done"));

        session.replaceExecutionPlan(created);
        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", created.planId()), created);
        session.replaceExecutionPlan(updated);
        appendRecord(session, TraceRecordType.PLAN_UPDATED, Instant.parse("2026-03-15T12:00:01Z"), Map.of("planId", updated.planId()), updated);

        assertThat(session.getExecutionPlan()).contains(updated);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.PLAN_CREATED, JournalEntryType.PLAN_UPDATED);
        assertThat(session.getJournalSnapshot().get(1).payload().get("tasks").get(0).get("status").textValue())
                .isEqualTo("COMPLETED");
    }

    @Test
    void storesAuthenticationAsRuntimeOnlySessionState() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        var authentication = UsernamePasswordAuthenticationToken.authenticated(
                "user",
                "pw",
                AuthorityUtils.createAuthorityList("ROLE_ALLOWED"));

        session.setAuthentication(authentication);

        assertThat(session.getAuthentication()).contains(authentication);
    }

    @Test
    void storesLastLinterOutcomeAndJournalsItSeparately() {
        LoomspanSession session = new LoomspanSession("session-1", "test.entry", 2);
        LinterOutcome outcome = new LinterOutcome(
                "lintedSkill",
                "regex",
                2,
                1,
                2,
                LinterOutcomeStatus.PASSED,
                "Return fenced YAML only.");

        session.setLastLinterOutcome(outcome);
        appendRecord(
                session,
                TraceRecordType.LINTER_RECORDED,
                Instant.parse("2026-03-15T12:00:03Z"),
                Map.of("skillName", outcome.skillName(), "status", outcome.status().name()),
                outcome);

        assertThat(session.getLastLinterOutcome()).contains(outcome);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.LINTER);
        assertThat(session.getJournalSnapshot().getFirst().payload().get("status").textValue()).isEqualTo("PASSED");
    }

    @Test
    void preservesFinalizedJournalAcrossRepeatedFinalizationAfterTraceDeletion() {
        LoomspanSession session = TestLoomspanSessions.withId(
                "session-repeat-finalize",
                "test.entry",
                2,
                null,
                TracePersistencePolicy.ONERROR,
                java.time.Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), java.time.ZoneOffset.UTC));
        ExecutionPlan plan = plan("plan-1");
        appendRecord(session, TraceRecordType.PLAN_CREATED, Instant.parse("2026-03-15T12:00:00Z"), Map.of("planId", plan.planId()), plan);

        TraceCompletion completion = new TraceCompletion(
                TraceOutcome.SUCCEEDED,
                com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty(),
                null,
                Map.of());
        session.finalizeTrace(completion);
        session.finalizeTrace(completion);

        assertThat(session.getExecutionTrace().completed()).isTrue();
        assertThat(session.getExecutionTrace().filePath()).isNull();
        assertThat(session.getExecutionJournal().getEntriesSnapshot())
                .extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.PLAN_CREATED);
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

    private static void appendError(LoomspanSession session, Instant timestamp, Object payload) {
        session.markTraceErrored();
        appendRecord(session, TraceRecordType.ERROR_RECORDED, timestamp, Map.of(), payload);
    }

    private static ExecutionPlan plan(String planId) {
        return new ExecutionPlan(
                planId,
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Plan", PlanTaskStatus.PENDING, null)));
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
