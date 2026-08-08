package com.lokiscale.loomspan.internal.runtime.observation.catalog;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;

import java.nio.file.Path;
import java.time.Instant;
import java.util.Objects;

public record FinalizedTraceCatalogEntry(
        long catalogOrdinal,
        String traceId,
        String sessionId,
        String entrySkill,
        TraceOutcome outcome,
        Instant finalizedAt,
        Instant publishedAt,
        Path artifactPath,
        long sizeBytes,
        TracePersistencePolicy persistencePolicy,
        Instant catalogExpiresAt,
        Instant applicationTraceExpiresAt,
        FinalizedTraceArtifact artifact)
{
    public FinalizedTraceCatalogEntry
    {
        if (catalogOrdinal <= 0)
        {
            throw new IllegalArgumentException("catalogOrdinal must be positive");
        }
        Objects.requireNonNull(traceId, "traceId must not be null");
        Objects.requireNonNull(sessionId, "sessionId must not be null");
        if (entrySkill == null || entrySkill.isBlank())
        {
            throw new IllegalArgumentException("entrySkill must not be blank");
        }
        Objects.requireNonNull(outcome, "outcome must not be null");
        Objects.requireNonNull(finalizedAt, "finalizedAt must not be null");
        Objects.requireNonNull(publishedAt, "publishedAt must not be null");
        artifactPath = Objects.requireNonNull(artifactPath, "artifactPath must not be null")
                .toAbsolutePath().normalize();
        if (sizeBytes < 0)
        {
            throw new IllegalArgumentException("sizeBytes must not be negative");
        }
        Objects.requireNonNull(persistencePolicy, "persistencePolicy must not be null");
        Objects.requireNonNull(catalogExpiresAt, "catalogExpiresAt must not be null");
        Objects.requireNonNull(applicationTraceExpiresAt, "applicationTraceExpiresAt must not be null");
        Objects.requireNonNull(artifact, "artifact must not be null");
    }
}
