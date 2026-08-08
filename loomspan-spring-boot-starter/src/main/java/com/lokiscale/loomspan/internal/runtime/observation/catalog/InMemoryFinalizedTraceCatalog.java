package com.lokiscale.loomspan.internal.runtime.observation.catalog;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention;

import java.io.IOException;
import java.nio.file.Files;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.Comparator;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;

public final class InMemoryFinalizedTraceCatalog implements FinalizedTraceCatalog
{
    private final Duration metadataTtl;
    private final Clock clock;
    private final ScheduledExecutorService executor;
    private final CompletionGraceRetention retention;
    private final ScheduledFuture<?> sweep;
    private final Map<String, FinalizedTraceCatalogEntry> entries = new ConcurrentHashMap<>();
    private final AtomicLong ordinal = new AtomicLong();
    private final AtomicBoolean closed = new AtomicBoolean();
    private long assignedHighWater;

    public InMemoryFinalizedTraceCatalog(
            Duration metadataTtl,
            Clock clock,
            CompletionGraceRetention retention)
    {
        this(metadataTtl, clock, retention,
                Executors.newSingleThreadScheduledExecutor(runnable ->
                        Thread.ofPlatform().daemon().name("loomspan-trace-catalog").unstarted(runnable)),
                sweepInterval(metadataTtl));
    }

    InMemoryFinalizedTraceCatalog(
            Duration metadataTtl,
            Clock clock,
            CompletionGraceRetention retention,
            ScheduledExecutorService executor,
            Duration sweepInterval)
    {
        this.metadataTtl = requirePositive(metadataTtl, "metadataTtl");
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
        this.retention = Objects.requireNonNull(retention, "retention must not be null");
        this.executor = Objects.requireNonNull(executor, "executor must not be null");
        Duration interval = requirePositive(sweepInterval, "sweepInterval");
        this.sweep = executor.scheduleAtFixedRate(
                this::purgeExpired,
                interval.toNanos(),
                interval.toNanos(),
                TimeUnit.NANOSECONDS);
    }

    @Override
    public synchronized FinalizedTraceCatalogEntry publish(FinalizedTraceArtifact artifact)
    {
        Objects.requireNonNull(artifact, "artifact must not be null");
        requireOpen();
        FinalizedTraceCatalogEntry existing = entries.get(artifact.traceId());
        if (existing != null)
        {
            if (existing.artifact().equals(artifact))
            {
                return existing;
            }
            throw new IllegalStateException("Conflicting finalized artifact for traceId '" + artifact.traceId() + "'");
        }
        if (!Files.isRegularFile(artifact.artifactPath()) || !Files.isReadable(artifact.artifactPath()))
        {
            throw new IllegalStateException("Finalized trace artifact is not an obtainable regular file");
        }
        Instant publishedAt = Instant.now(clock);
        Instant catalogExpiresAt;
        try
        {
            catalogExpiresAt = publishedAt.plus(metadataTtl);
        }
        catch (RuntimeException ex)
        {
            throw new IllegalArgumentException("publication time plus metadataTtl must be representable", ex);
        }
        Instant effectiveExpiresAt = artifact.artifactExpiresAt() != null
                && artifact.artifactExpiresAt().isBefore(catalogExpiresAt)
                ? artifact.artifactExpiresAt()
                : catalogExpiresAt;
        if (!effectiveExpiresAt.isAfter(publishedAt))
        {
            throw new IllegalStateException("Finalized trace artifact is already expired");
        }
        long assigned = assignOrdinal();
        FinalizedTraceCatalogEntry entry = new FinalizedTraceCatalogEntry(
                assigned,
                artifact.traceId(),
                artifact.sessionId(),
                artifact.entrySkill(),
                artifact.outcome(),
                artifact.finalizedAt(),
                publishedAt,
                artifact.artifactPath(),
                artifact.sizeBytes(),
                artifact.persistencePolicy(),
                catalogExpiresAt,
                effectiveExpiresAt,
                artifact);
        entries.put(entry.traceId(), entry);
        return entry;
    }

    @Override
    public Optional<FinalizedTraceCatalogEntry> find(String traceId)
    {
        Objects.requireNonNull(traceId, "traceId must not be null");
        FinalizedTraceCatalogEntry entry = entries.get(traceId);
        if (entry == null || expired(entry, Instant.now(clock)))
        {
            if (entry != null)
            {
                entries.remove(traceId, entry);
            }
            return Optional.empty();
        }
        return Optional.of(entry);
    }

    @Override
    public Optional<ArtifactAcquisition> acquire(String traceId) throws IOException
    {
        Objects.requireNonNull(traceId, "traceId must not be null");
        FinalizedTraceCatalogEntry entry = entries.get(traceId);
        Instant acquisitionStartedAt = Instant.now(clock);
        if (closed.get() || entry == null || expired(entry, acquisitionStartedAt))
        {
            if (entry != null)
            {
                entries.remove(traceId, entry);
            }
            return Optional.empty();
        }
        Optional<CompletionGraceRetention.ArtifactLease> lease = retention.acquire(entry.artifact());
        if (lease.isEmpty())
        {
            return Optional.empty();
        }
        return Optional.of(new ArtifactAcquisition(entry.traceId(), entry.sizeBytes(), lease.orElseThrow()));
    }

    @Override
    public TraceCatalogSlice list(long highWaterOrdinal, long beforeOrdinal, int limit)
    {
        if (highWaterOrdinal < 0 || beforeOrdinal < 0)
        {
            throw new IllegalArgumentException("ordinal positions must not be negative");
        }
        if (limit <= 0)
        {
            throw new IllegalArgumentException("limit must be positive");
        }
        long highWater = highWaterOrdinal == 0 ? assignedHighWater() : highWaterOrdinal;
        long before = beforeOrdinal == 0 ? Long.MAX_VALUE : beforeOrdinal;
        Instant now = Instant.now(clock);
        return new TraceCatalogSlice(
                highWater,
                entries.values().stream()
                        .filter(entry -> entry.catalogOrdinal() <= highWater)
                        .filter(entry -> entry.catalogOrdinal() < before)
                        .filter(entry -> !expired(entry, now))
                        .sorted(Comparator.comparingLong(FinalizedTraceCatalogEntry::catalogOrdinal).reversed())
                        .limit(limit)
                .toList());
    }

    @Override
    public int catalogedTraceCount()
    {
        Instant now = Instant.now(clock);
        return Math.toIntExact(entries.values().stream().filter(entry -> !expired(entry, now)).count());
    }

    void purgeExpired()
    {
        Instant now = Instant.now(clock);
        entries.entrySet().removeIf(entry -> expired(entry.getValue(), now));
    }

    @Override
    public synchronized void close()
    {
        if (!closed.compareAndSet(false, true))
        {
            return;
        }
        sweep.cancel(false);
        executor.shutdownNow();
        entries.clear();
    }

    private static boolean expired(FinalizedTraceCatalogEntry entry, Instant now)
    {
        return !now.isBefore(entry.applicationTraceExpiresAt());
    }

    private synchronized long assignOrdinal()
    {
        long value = ordinal.updateAndGet(current ->
        {
            if (current == Long.MAX_VALUE)
            {
                throw new IllegalStateException("Catalog ordinal exhausted");
            }
            return current + 1;
        });
        if (value <= 0 || value <= assignedHighWater)
        {
            throw new IllegalStateException("Catalog ordinal must be positive, unique, and strictly increasing");
        }
        assignedHighWater = value;
        return value;
    }

    private synchronized long assignedHighWater()
    {
        return assignedHighWater;
    }

    private void requireOpen()
    {
        if (closed.get())
        {
            throw new IllegalStateException("Finalized trace catalog is closed");
        }
    }

    private static Duration sweepInterval(Duration ttl)
    {
        Duration valid = requirePositive(ttl, "metadataTtl");
        Duration interval = valid.compareTo(Duration.ofMinutes(1)) < 0 ? valid : Duration.ofMinutes(1);
        return interval.compareTo(Duration.ofSeconds(1)) < 0 ? Duration.ofSeconds(1) : interval;
    }

    private static Duration requirePositive(Duration value, String name)
    {
        Objects.requireNonNull(value, name + " must not be null");
        if (value.isZero() || value.isNegative())
        {
            throw new IllegalArgumentException(name + " must be positive");
        }
        try
        {
            value.toNanos();
        }
        catch (ArithmeticException ex)
        {
            throw new IllegalArgumentException(name + " is too large", ex);
        }
        return value;
    }
}
