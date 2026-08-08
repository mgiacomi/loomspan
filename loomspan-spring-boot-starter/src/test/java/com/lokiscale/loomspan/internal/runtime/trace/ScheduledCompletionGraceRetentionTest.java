package com.lokiscale.loomspan.internal.runtime.trace;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneId;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.doReturn;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class ScheduledCompletionGraceRetentionTest
{
    @TempDir
    Path tempDir;

    @Test
    void zeroGraceDeletesSynchronouslyAndNonzeroCloseCancelsWithoutDeleting() throws Exception
    {
        Path immediate = Files.writeString(tempDir.resolve("immediate"), "trace");
        try (ScheduledCompletionGraceRetention retention =
                     new ScheduledCompletionGraceRetention(Duration.ZERO))
        {
            assertThat(retention.retainOrDelete(
                    immediate, Instant.parse("2026-07-24T12:00:00Z"), "trace", "session")).isEmpty();
        }
        assertThat(immediate).doesNotExist();

        Path held = Files.writeString(tempDir.resolve("held"), "trace");
        ScheduledCompletionGraceRetention retention =
                new ScheduledCompletionGraceRetention(Duration.ofHours(1));
        Instant finalizedAt = Instant.now();
        assertThat(retention.retainOrDelete(
                held, finalizedAt, "trace", "session"))
                .contains(new CompletionGraceRetention.RetainedArtifact(
                        finalizedAt.plus(Duration.ofHours(1)),
                        Files.size(held)));
        retention.close();
        assertThat(held).exists();
    }

    @Test
    void rejectsNegativeGrace()
    {
        assertThatThrownBy(() -> new ScheduledCompletionGraceRetention(Duration.ofSeconds(-1)))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void leaseOpenedBeforeDeadlineDefersDueDeletionUntilClose() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-26T12:00:00Z"));
        ScheduledExecutorService executor = mock(ScheduledExecutorService.class);
        @SuppressWarnings("unchecked")
        ScheduledFuture<Object> future = mock(ScheduledFuture.class);
        doReturn(future).when(executor).schedule(
                any(Runnable.class), anyLong(), eq(java.util.concurrent.TimeUnit.NANOSECONDS));
        Path file = Files.writeString(tempDir.resolve("leased.ndjson"), "{}\n");
        ScheduledCompletionGraceRetention retention =
                new ScheduledCompletionGraceRetention(Duration.ofMinutes(1), clock, executor);
        Instant finalizedAt = clock.instant();
        var retained = retention.retainOrDelete(file, finalizedAt, "trace", "session").orElseThrow();
        FinalizedTraceArtifact artifact = new FinalizedTraceArtifact(
                "trace", "session", "test.entry", TraceOutcome.SUCCEEDED, finalizedAt, file,
                retained.sizeBytes(), TracePersistencePolicy.NEVER, retained.expiresAt());
        CompletionGraceRetention.ArtifactLease lease = retention.acquire(artifact).orElseThrow();
        var task = org.mockito.ArgumentCaptor.forClass(Runnable.class);
        verify(executor).schedule(task.capture(), anyLong(), eq(java.util.concurrent.TimeUnit.NANOSECONDS));

        clock.advance(Duration.ofMinutes(1));
        task.getValue().run();

        assertThat(file).exists();
        assertThat(retention.acquire(artifact)).isEmpty();
        assertThat(lease.input().readAllBytes()).isEqualTo("{}\n".getBytes(java.nio.charset.StandardCharsets.UTF_8));
        lease.close();
        lease.close();
        assertThat(file).doesNotExist();
        assertThat(retention.retainedArtifactCount()).isZero();
        retention.close();
    }

    @Test
    void nonExpiringArtifactLeaseNeverTransfersDeletionOwnership() throws Exception
    {
        Path file = Files.writeString(tempDir.resolve("always.ndjson"), "{}\n");
        try (ScheduledCompletionGraceRetention retention =
                     new ScheduledCompletionGraceRetention(Duration.ofMinutes(1)))
        {
            FinalizedTraceArtifact artifact = new FinalizedTraceArtifact(
                    "always", "session", "test.entry", TraceOutcome.SUCCEEDED, Instant.now(), file,
                    Files.size(file), TracePersistencePolicy.ALWAYS, null);
            try (CompletionGraceRetention.ArtifactLease lease = retention.acquire(artifact).orElseThrow())
            {
                assertThat(lease.sizeBytes()).isEqualTo(Files.size(file));
            }
        }
        assertThat(file).exists();
    }

    private static final class MutableClock extends Clock
    {
        private Instant instant;

        private MutableClock(Instant instant)
        {
            this.instant = instant;
        }

        void advance(Duration duration)
        {
            instant = instant.plus(duration);
        }

        @Override public ZoneId getZone() { return ZoneId.of("UTC"); }
        @Override public Clock withZone(ZoneId zone) { return this; }
        @Override public Instant instant() { return instant; }
    }
}
