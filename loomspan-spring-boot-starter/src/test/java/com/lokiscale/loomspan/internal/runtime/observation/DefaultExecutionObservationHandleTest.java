package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.runtime.observation.catalog.InMemoryFinalizedTraceCatalog;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.springframework.boot.test.system.CapturedOutput;
import org.springframework.boot.test.system.OutputCaptureExtension;

import java.time.Instant;
import java.time.Clock;
import java.time.Duration;
import java.time.ZoneOffset;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.Executors;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

@ExtendWith(OutputCaptureExtension.class)
class DefaultExecutionObservationHandleTest
{
    @org.junit.jupiter.api.io.TempDir
    Path tempDir;

    @Test
    void signalsAfterSuccessfulPublicationAndFirstFailClosedTransition()
    {
        AtomicInteger activitySignals = new AtomicInteger();
        AtomicInteger unavailableSignals = new AtomicInteger();
        LiveActivitySignal signal = new LiveActivitySignal()
        {
            @Override
            public void activityAvailable()
            {
                activitySignals.incrementAndGet();
            }

            @Override
            public void liveUnavailable()
            {
                unavailableSignals.incrementAndGet();
            }
        };
        LiveMonitoringAvailability availability = new LiveMonitoringAvailability();
        InMemoryActiveExecutionRegistry registry = new InMemoryActiveExecutionRegistry();
        DefaultExecutionObservationHandle successful = new DefaultExecutionObservationHandle("session", "test.entry", new LiveActivityProjector(), registry,
                new InMemoryActivityReplayBuffer(), availability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(), signal);

        successful.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));

        assertThat(activitySignals).hasValue(1);
        assertThat(registry.find("session").orElseThrow().entrySkill()).isEqualTo("test.entry");
        DefaultExecutionObservationHandle failing = new DefaultExecutionObservationHandle("other", "test.entry", new LiveActivityProjector()
                {
                    @Override
                    Projection project(ExecutionProjectionState state, TraceRecord record)
                    {
                        throw new IllegalStateException("not logged");
                    }
                },
                new InMemoryActiveExecutionRegistry(),
                new InMemoryActivityReplayBuffer(),
                availability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                signal);
        failing.recordAppended(new TraceRecord(
                "trace-other", "other", 1, Instant.parse("2026-07-24T12:00:00Z"),
                TraceRecordType.TRACE_STARTED, null, null, null, null, "thread", Map.of(), null));
        failing.recordAppended(new TraceRecord(
                "trace-other", "other", 2, Instant.parse("2026-07-24T12:00:01Z"),
                TraceRecordType.TRACE_STARTED, null, null, null, null, "thread", Map.of(), null));

        assertThat(unavailableSignals).hasValue(1);
    }

    @Test
    void catalogsBeforePublishingAvailableTerminal() throws Exception
    {
        Instant now = Instant.parse("2026-07-24T12:00:00Z");
        Clock clock = Clock.fixed(now, ZoneOffset.UTC);
        Path artifactPath = Files.writeString(tempDir.resolve("trace.ndjson"), "{}\n");
        try (InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(
                             Duration.ofHours(1), clock,
                             org.mockito.Mockito.mock(
                                     com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class)))
        {
            DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory(
                    new LiveActivityProjector(),
                    new InMemoryActiveExecutionRegistry(),
                    new InMemoryActivityReplayBuffer(),
                    new LiveMonitoringAvailability(),
                    catalog,
                    LiveActivitySignal.NO_OP);
            ExecutionObservationHandle handle = factory.create("session", "test.entry");
            handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
            handle.recordAppended(record(
                    TraceRecordType.TRACE_COMPLETED,
                    2,
                    Map.of("outcome", "SUCCEEDED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));
            FinalizedTraceArtifact artifact = new FinalizedTraceArtifact(
                    "trace", "session", "test.entry", TraceOutcome.SUCCEEDED, now, artifactPath,
                    Files.size(artifactPath), TracePersistencePolicy.ALWAYS, null);

            handle.close(new ObservationCompletionDisposition(
                    ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                    TraceOutcome.SUCCEEDED,
                    now,
                    Optional.of(artifact)));

            assertThat(catalog.find("trace")).isPresent();
            assertThat(factory.replayBuffer().replayAfter(0, 10).activities().getLast().details())
                    .containsEntry("applicationTraceAvailability", "AVAILABLE")
                    .containsEntry("applicationTraceExpiresAt", now.plus(Duration.ofHours(1)).toString())
                    .doesNotContainKey("artifactPath");
            assertThat(factory.registry().activeCount()).isZero();
        }
    }

    @Test
    void reportsUnavailableWhenArtifactExpiredBeforePublication() throws Exception
    {
        Instant now = Instant.parse("2026-07-24T12:00:00Z");
        Clock clock = Clock.fixed(now, ZoneOffset.UTC);
        Path artifactPath = Files.writeString(tempDir.resolve("expired-trace.ndjson"), "{}\n");
        try (InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(
                             Duration.ofHours(1), clock,
                             org.mockito.Mockito.mock(
                                     com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class)))
        {
            DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory(
                    new LiveActivityProjector(),
                    new InMemoryActiveExecutionRegistry(),
                    new InMemoryActivityReplayBuffer(),
                    new LiveMonitoringAvailability(),
                    catalog,
                    LiveActivitySignal.NO_OP);
            ExecutionObservationHandle handle = factory.create("session", "test.entry");
            handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
            handle.recordAppended(record(
                    TraceRecordType.TRACE_COMPLETED,
                    2,
                    Map.of("outcome", "SUCCEEDED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));
            FinalizedTraceArtifact expired = new FinalizedTraceArtifact(
                    "trace",
                    "session",
                    "test.entry",
                    TraceOutcome.SUCCEEDED,
                    now.minusSeconds(30),
                    artifactPath,
                    Files.size(artifactPath),
                    TracePersistencePolicy.NEVER,
                    now);

            handle.close(new ObservationCompletionDisposition(
                    ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                    TraceOutcome.SUCCEEDED,
                    now,
                    Optional.of(expired)));

            assertThat(catalog.find("trace")).isEmpty();
            assertThat(factory.replayBuffer().replayAfter(0, 10).activities().getLast().details())
                    .containsEntry("applicationTraceAvailability", "UNAVAILABLE")
                    .containsEntry("applicationTraceUnavailableReason", "CATALOG_PUBLICATION_FAILED")
                    .doesNotContainKey("applicationTraceExpiresAt");
            assertThat(factory.availability().isAvailable()).isTrue();
            assertThat(factory.registry().activeCount()).isZero();
        }
    }

    @Test
    void holdsCanonicalCompletionUntilCoreSuccessClose()
    {
        DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory();
        ExecutionObservationHandle handle = factory.create("session", "test.entry");
        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
        handle.recordAppended(record(
                TraceRecordType.TRACE_COMPLETED,
                2,
                Map.of("outcome", "SUCCEEDED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));

        assertThat(factory.replayBuffer().replayAfter(0, 10).activities())
                .extracting(ExecutionActivity::kind)
                .containsExactly(ExecutionActivityKind.TRACE_STARTED);
        assertThat(factory.registry().activeCount()).isEqualTo(1);

        handle.close(disposition(
                ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                TraceOutcome.SUCCEEDED));

        assertThat(factory.replayBuffer().replayAfter(0, 10).activities())
                .extracting(ExecutionActivity::kind)
                .containsExactly(
                        ExecutionActivityKind.TRACE_STARTED,
                        ExecutionActivityKind.TRACE_COMPLETED);
        assertThat(factory.registry().activeCount()).isZero();
    }

    @Test
    void discardsHeldCompletionAndPublishesObservationEndedOnCoreFailure()
    {
        DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory();
        ExecutionObservationHandle handle = factory.create("session", "test.entry");
        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
        handle.recordAppended(record(
                TraceRecordType.TRACE_COMPLETED,
                2,
                Map.of("outcome", "FAILED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));

        handle.close(disposition(
                ObservationCompletionDisposition.Status.CORE_FINALIZATION_FAILED,
                TraceOutcome.FAILED));

        List<ExecutionActivity> activities = factory.replayBuffer().replayAfter(0, 10).activities();
        assertThat(activities).extracting(ExecutionActivity::kind)
                .containsExactly(
                        ExecutionActivityKind.TRACE_STARTED,
                        ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED);
        assertThat(activities.getLast().canonicalSequence()).isNull();
        assertThat(activities.getLast().details())
                .containsEntry("reason", "CORE_FINALIZATION_FAILED")
                .containsEntry("outcome", "FAILED");
        assertThat(activities.getLast().executionStatus()).isEqualTo("FAILED");
        assertThat(activities.getLast().retainedWeight())
                .isEqualTo(expectedRetainedWeight(activities.getLast()));
        assertThat(factory.registry().activeCount()).isZero();
    }

    @Test
    void containsReplayFailureAndFailsClosedWithoutThrowing()
    {
        LiveMonitoringAvailability availability = new LiveMonitoringAvailability();
        ActivityReplayBuffer throwing = new ActivityReplayBuffer()
        {
            @Override
            public ExecutionActivity append(ExecutionActivity activity)
            {
                throw new IllegalStateException("SECRET");
            }

            @Override
            public long currentCursor()
            {
                return 0;
            }

            @Override
            public ReplayResult replayAfter(long cursor, int limit)
            {
                return new ReplayResult(ReplayResult.Status.EMPTY, 0, List.of());
            }
        };
        DefaultExecutionObservationHandle handle = new DefaultExecutionObservationHandle("session", "test.entry", new LiveActivityProjector(),
                new InMemoryActiveExecutionRegistry(),
                throwing,
                availability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                LiveActivitySignal.NO_OP);

        assertThatCode(() -> handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of())))
                .doesNotThrowAnyException();
        assertThat(availability.isAvailable()).isFalse();
        assertThat(availability.firstFailure().orElseThrow().operation())
                .isEqualTo("REPLAY_PUBLICATION_FAILED");
    }

    @Test
    void closesExactlyOnceUnderConcurrentConflictingCalls() throws Exception
    {
        DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory();
        ExecutionObservationHandle handle = factory.create("session", "test.entry");
        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
        handle.recordAppended(record(
                TraceRecordType.TRACE_COMPLETED,
                2,
                Map.of("outcome", "SUCCEEDED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));
        CountDownLatch start = new CountDownLatch(1);

        try (var executor = Executors.newVirtualThreadPerTaskExecutor())
        {
            var success = executor.submit(() -> {
                start.await();
                handle.close(disposition(
                        ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                        TraceOutcome.SUCCEEDED));
                return null;
            });
            var failure = executor.submit(() -> {
                start.await();
                handle.close(disposition(
                        ObservationCompletionDisposition.Status.CORE_FINALIZATION_FAILED,
                        TraceOutcome.FAILED));
                return null;
            });
            start.countDown();
            success.get();
            failure.get();
        }

        List<ExecutionActivityKind> terminal = factory.replayBuffer().replayAfter(0, 10).activities().stream()
                .map(ExecutionActivity::kind)
                .filter(kind -> kind == ExecutionActivityKind.TRACE_COMPLETED
                        || kind == ExecutionActivityKind.EXECUTION_OBSERVATION_ENDED)
                .toList();
        assertThat(terminal).hasSize(1);
        assertThat(factory.registry().activeCount()).isZero();
    }

    @Test
    void containsProjectorAndRegistryFailuresAndFailsClosed()
    {
        LiveMonitoringAvailability projectorAvailability = new LiveMonitoringAvailability();
        DefaultExecutionObservationHandle projectorFailure = new DefaultExecutionObservationHandle("session", "test.entry", new LiveActivityProjector()
                {
                    @Override
                    Projection project(ExecutionProjectionState state, TraceRecord record)
                    {
                        throw new IllegalArgumentException("SECRET-PROJECTOR");
                    }
                },
                new InMemoryActiveExecutionRegistry(),
                new InMemoryActivityReplayBuffer(),
                projectorAvailability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                LiveActivitySignal.NO_OP);
        assertThatCode(() -> projectorFailure.recordAppended(
                record(TraceRecordType.TRACE_STARTED, 1, Map.of()))).doesNotThrowAnyException();
        assertThat(projectorAvailability.firstFailure().orElseThrow().operation())
                .isEqualTo("PROJECTION_FAILED");

        LiveMonitoringAvailability registryAvailability = new LiveMonitoringAvailability();
        ActiveExecutionRegistry throwingRegistry = new ActiveExecutionRegistry()
        {
            @Override
            public ActiveExecutionSnapshot replace(ActiveExecutionSnapshot snapshot)
            {
                throw new IllegalStateException("SECRET-REGISTRY");
            }

            @Override
            public Optional<ActiveExecutionSnapshot> find(String sessionId)
            {
                return Optional.empty();
            }

            @Override
            public boolean remove(String sessionId)
            {
                return false;
            }

            @Override
            public int activeCount()
            {
                return 0;
            }

            @Override
            public long highestOrdinal()
            {
                return 0;
            }

            @Override
            public List<ActiveExecutionSnapshot> newestFirst(long highWaterMark, long beforeOrdinal, int limit)
            {
                return List.of();
            }
        };
        DefaultExecutionObservationHandle registryFailure = new DefaultExecutionObservationHandle("session", "test.entry", new LiveActivityProjector(),
                throwingRegistry,
                new InMemoryActivityReplayBuffer(),
                registryAvailability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                LiveActivitySignal.NO_OP);
        assertThatCode(() -> registryFailure.recordAppended(
                record(TraceRecordType.TRACE_STARTED, 1, Map.of()))).doesNotThrowAnyException();
        assertThat(registryAvailability.firstFailure().orElseThrow().operation())
                .isEqualTo("REGISTRY_UPDATE_FAILED");
    }

    @Test
    void terminalPublicationFailureStillRemovesActiveEntry()
    {
        InMemoryActiveExecutionRegistry registry = new InMemoryActiveExecutionRegistry();
        LiveMonitoringAvailability availability = new LiveMonitoringAvailability();
        InMemoryActivityReplayBuffer delegate = new InMemoryActivityReplayBuffer();
        ActivityReplayBuffer failSecond = new ActivityReplayBuffer()
        {
            private int publications;

            @Override
            public ExecutionActivity append(ExecutionActivity activity)
            {
                if (++publications == 2)
                {
                    throw new IllegalStateException("SECRET-TERMINAL");
                }
                return delegate.append(activity);
            }

            @Override
            public long currentCursor()
            {
                return delegate.currentCursor();
            }

            @Override
            public ReplayResult replayAfter(long cursor, int limit)
            {
                return delegate.replayAfter(cursor, limit);
            }
        };
        DefaultExecutionObservationHandle handle = new DefaultExecutionObservationHandle("session", "test.entry", new LiveActivityProjector(), registry, failSecond, availability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                LiveActivitySignal.NO_OP);
        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
        handle.recordAppended(record(
                TraceRecordType.TRACE_COMPLETED,
                2,
                Map.of("outcome", "SUCCEEDED", "sessionUsageSnapshot", SessionUsageSnapshot.empty())));

        assertThatCode(() -> handle.close(disposition(
                ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                TraceOutcome.SUCCEEDED))).doesNotThrowAnyException();
        assertThat(registry.activeCount()).isZero();
        assertThat(availability.firstFailure().orElseThrow().operation())
                .isEqualTo("TERMINAL_PUBLICATION_FAILED");
    }

    @Test
    void missingHeldCompletionOnSuccessFailsClosedAndRemovesEntry()
    {
        DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory();
        ExecutionObservationHandle handle = factory.create("session", "test.entry");
        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));

        handle.close(disposition(
                ObservationCompletionDisposition.Status.CORE_FINALIZATION_SUCCEEDED,
                TraceOutcome.SUCCEEDED));

        assertThat(factory.registry().activeCount()).isZero();
        assertThat(factory.availability().firstFailure().orElseThrow().operation())
                .isEqualTo("TERMINAL_PUBLICATION_FAILED");
    }

    @Test
    void logsOneSanitizedDiagnosticOnFirstFailure(CapturedOutput output)
    {
        LiveMonitoringAvailability availability = new LiveMonitoringAvailability();
        DefaultExecutionObservationHandle handle = new DefaultExecutionObservationHandle("session-safe-id", "test.entry", new LiveActivityProjector()
                {
                    @Override
                    Projection project(ExecutionProjectionState state, TraceRecord record)
                    {
                        throw new IllegalArgumentException("SECRET-MESSAGE-CONTENT");
                    }
                },
                new InMemoryActiveExecutionRegistry(),
                new InMemoryActivityReplayBuffer(),
                availability,
                DefaultExecutionObservationHandleFactory.unavailableCatalog(),
                LiveActivitySignal.NO_OP);

        handle.recordAppended(record(TraceRecordType.TRACE_STARTED, 1, Map.of()));
        handle.recordAppended(record(TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED, 2, Map.of()));

        assertThat(output.getOut())
                .contains("operation=PROJECTION_FAILED")
                .contains("sessionId=session-safe-id")
                .contains("traceId=trace")
                .contains("exceptionClass=java.lang.IllegalArgumentException")
                .doesNotContain("SECRET-MESSAGE-CONTENT");
        assertThat(output.getOut().split("Live monitoring unavailable", -1)).hasSize(2);
    }

    private TraceRecord record(TraceRecordType type, long sequence, Map<String, Object> metadata)
    {
        return new TraceRecord(
                "trace", "session", sequence, Instant.parse("2026-07-24T12:00:00Z"), type,
                null, null, null, null, "thread", metadata, null);
    }

    private ObservationCompletionDisposition disposition(
            ObservationCompletionDisposition.Status status,
            TraceOutcome outcome)
    {
        return new ObservationCompletionDisposition(
                status, outcome, Instant.parse("2026-07-24T12:01:00Z"));
    }

    private static int expectedRetainedWeight(ExecutionActivity activity)
    {
        int weight = 128
                + ExecutionObservationLimits.utf8Weight(activity.sessionId())
                + ExecutionObservationLimits.utf8Weight(activity.traceId())
                + ExecutionObservationLimits.utf8Weight(activity.frameId())
                + ExecutionObservationLimits.utf8Weight(activity.parentFrameId())
                + ExecutionObservationLimits.utf8Weight(activity.route())
                + ExecutionObservationLimits.utf8Weight(activity.executionStatus())
                + ExecutionObservationLimits.utf8Weight(activity.kind().name())
                + ExecutionObservationLimits.utf8Weight(activity.summary());
        for (Map.Entry<String, Object> entry : activity.details().entrySet())
        {
            weight += ExecutionObservationLimits.utf8Weight(entry.getKey())
                    + ExecutionObservationLimits.utf8Weight(String.valueOf(entry.getValue())) + 8;
        }
        return Math.max(1, weight);
    }
}
