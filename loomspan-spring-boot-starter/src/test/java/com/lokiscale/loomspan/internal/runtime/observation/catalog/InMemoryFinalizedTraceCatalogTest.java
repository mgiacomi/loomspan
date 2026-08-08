package com.lokiscale.loomspan.internal.runtime.observation.catalog;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneId;
import java.util.concurrent.CountDownLatch;
import com.lokiscale.loomspan.internal.runtime.trace.ScheduledCompletionGraceRetention;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class InMemoryFinalizedTraceCatalogTest
{
    @TempDir
    Path tempDir;

    @Test
    void publishesTraversesIdempotentlyAndExpiresWithoutDeletingArtifact() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-24T12:00:00Z"));
        Path first = Files.writeString(tempDir.resolve("first.ndjson"), "{}\n");
        Path second = Files.writeString(tempDir.resolve("second.ndjson"), "{}\n");
        try (InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(
                             Duration.ofMinutes(5), clock,
                             org.mockito.Mockito.mock(
                                     com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class)))
        {
            FinalizedTraceArtifact a = artifact("a", first, clock.instant(), null);
            FinalizedTraceCatalogEntry firstEntry = catalog.publish(a);
            assertThat(catalog.publish(a)).isSameAs(firstEntry);
            FinalizedTraceCatalogEntry secondEntry =
                    catalog.publish(artifact("b", second, clock.instant(), null));
            assertThat(catalog.catalogedTraceCount()).isEqualTo(2);

            TraceCatalogSlice page = catalog.list(0, 0, 1);
            assertThat(page.highWaterOrdinal()).isEqualTo(secondEntry.catalogOrdinal());
            assertThat(page.entries()).extracting(FinalizedTraceCatalogEntry::traceId).containsExactly("b");
            assertThat(catalog.list(page.highWaterOrdinal(), secondEntry.catalogOrdinal(), 10).entries())
                    .extracting(FinalizedTraceCatalogEntry::traceId).containsExactly("a");

            clock.advance(Duration.ofMinutes(5));
            assertThat(catalog.catalogedTraceCount()).isZero();
            assertThat(catalog.find("a")).isEmpty();
            assertThat(catalog.list(page.highWaterOrdinal(), 0, 10).entries()).isEmpty();
            assertThat(first).exists();
            assertThat(second).exists();
        }
        assertThat(first).exists();
        assertThat(second).exists();
    }

    @Test
    void usesEarlierCoreExpirationAndRejectsConflictsOrMissingFiles() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-24T12:00:00Z"));
        Path file = Files.writeString(tempDir.resolve("trace.ndjson"), "{}\n");
        try (InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(
                             Duration.ofHours(1), clock,
                             org.mockito.Mockito.mock(
                                     com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class)))
        {
            Instant coreExpiry = clock.instant().plusSeconds(30);
            FinalizedTraceCatalogEntry entry =
                    catalog.publish(artifact("trace", file, clock.instant(), coreExpiry));
            assertThat(entry.applicationTraceExpiresAt()).isEqualTo(coreExpiry);
            assertThatThrownBy(() -> catalog.publish(
                    artifact("trace", file, clock.instant().plusSeconds(1), coreExpiry)))
                    .isInstanceOf(IllegalStateException.class);
            assertThatThrownBy(() -> catalog.publish(
                    artifact("missing", tempDir.resolve("missing"), clock.instant(), null)))
                    .isInstanceOf(IllegalStateException.class);
        }
    }

    @Test
    void rejectsArtifactExpiredAtPublicationTime() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-24T12:00:00Z"));
        Path file = Files.writeString(tempDir.resolve("expired.ndjson"), "{}\n");
        try (InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(
                             Duration.ofHours(1), clock,
                             org.mockito.Mockito.mock(
                                     com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class)))
        {
            FinalizedTraceArtifact expired = artifact(
                    "expired",
                    file,
                    clock.instant().minusSeconds(30),
                    clock.instant());

            assertThatThrownBy(() -> catalog.publish(expired))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("already expired");
            assertThat(catalog.find("expired")).isEmpty();
            assertThat(catalog.list(0, 0, 10).entries()).isEmpty();
        }
    }

    @Test
    void closeSerializesWithPublicationAndClearsEntries() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-24T12:00:00Z"));
        Path file = Files.writeString(tempDir.resolve("trace.ndjson"), "{}\n");
        InMemoryFinalizedTraceCatalog catalog =
                new InMemoryFinalizedTraceCatalog(
                        Duration.ofHours(1), clock,
                        org.mockito.Mockito.mock(
                                com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention.class));
        catalog.publish(artifact("trace", file, clock.instant(), null));
        CountDownLatch started = new CountDownLatch(1);
        Thread closer;

        synchronized (catalog)
        {
            closer = Thread.ofPlatform().start(() ->
            {
                started.countDown();
                catalog.close();
            });
            started.await();
            long deadline = System.nanoTime() + Duration.ofSeconds(2).toNanos();
            while (closer.getState() != Thread.State.BLOCKED && System.nanoTime() < deadline)
            {
                Thread.onSpinWait();
            }
            assertThat(closer.getState()).isEqualTo(Thread.State.BLOCKED);
            assertThat(catalog.find("trace")).isPresent();
        }

        closer.join(Duration.ofSeconds(2));
        assertThat(closer.isAlive()).isFalse();
        assertThat(catalog.find("trace")).isEmpty();
        assertThatThrownBy(() -> catalog.publish(artifact("late", file, clock.instant(), null)))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("closed");
    }

    @Test
    void acquiresPathFreeMetadataAndCoreLeaseBeforeEffectiveExpiry() throws Exception
    {
        MutableClock clock = new MutableClock(Instant.parse("2026-07-24T12:00:00Z"));
        Path file = Files.writeString(tempDir.resolve("acquired.ndjson"), "{}\n");
        try (ScheduledCompletionGraceRetention retention =
                     new ScheduledCompletionGraceRetention(Duration.ofMinutes(5));
             InMemoryFinalizedTraceCatalog catalog =
                     new InMemoryFinalizedTraceCatalog(Duration.ofMinutes(5), clock, retention))
        {
            catalog.publish(artifact("acquired", file, clock.instant(), null));
            FinalizedTraceCatalog.ArtifactAcquisition acquisition =
                    catalog.acquire("acquired").orElseThrow();

            assertThat(acquisition.traceId()).isEqualTo("acquired");
            assertThat(acquisition.sizeBytes()).isEqualTo(Files.size(file));
            assertThat(acquisition.getClass().getRecordComponents())
                    .extracting(java.lang.reflect.RecordComponent::getName)
                    .containsExactly("traceId", "sizeBytes", "lease");
            assertThat(acquisition.lease().input().readAllBytes())
                    .isEqualTo("{}\n".getBytes(java.nio.charset.StandardCharsets.UTF_8));
            acquisition.lease().close();

            clock.advance(Duration.ofMinutes(5));
            assertThat(catalog.acquire("acquired")).isEmpty();
            assertThat(file).exists();
        }
    }

    private FinalizedTraceArtifact artifact(String id, Path path, Instant finalizedAt, Instant expiresAt)
            throws Exception
    {
        return new FinalizedTraceArtifact(
                id, "session-" + id, "test.entry", TraceOutcome.SUCCEEDED, finalizedAt, path,
                Files.exists(path) ? Files.size(path) : 0,
                TracePersistencePolicy.ALWAYS, expiresAt);
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

        @Override
        public ZoneId getZone()
        {
            return ZoneId.of("UTC");
        }

        @Override
        public Clock withZone(ZoneId zone)
        {
            return this;
        }

        @Override
        public Instant instant()
        {
            return instant;
        }
    }
}
