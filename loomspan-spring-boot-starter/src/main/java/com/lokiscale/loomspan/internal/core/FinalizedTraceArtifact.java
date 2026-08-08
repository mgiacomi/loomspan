package com.lokiscale.loomspan.internal.core;

import org.springframework.lang.Nullable;

import java.nio.file.Path;
import java.time.Instant;
import java.util.Objects;

public record FinalizedTraceArtifact(
        String traceId,
        String sessionId,
        String entrySkill,
        TraceOutcome outcome,
        Instant finalizedAt,
        Path artifactPath,
        long sizeBytes,
        TracePersistencePolicy persistencePolicy,
        @Nullable Instant artifactExpiresAt)
{
    public FinalizedTraceArtifact
    {
        traceId = requireNonBlank(traceId, "traceId");
        sessionId = requireNonBlank(sessionId, "sessionId");
        entrySkill = requireNonBlank(entrySkill, "entrySkill");
        Objects.requireNonNull(outcome, "outcome must not be null");
        Objects.requireNonNull(finalizedAt, "finalizedAt must not be null");
        artifactPath = Objects.requireNonNull(artifactPath, "artifactPath must not be null").toAbsolutePath().normalize();
        if (sizeBytes < 0)
        {
            throw new IllegalArgumentException("sizeBytes must not be negative");
        }
        Objects.requireNonNull(persistencePolicy, "persistencePolicy must not be null");
        if (artifactExpiresAt != null && !artifactExpiresAt.isAfter(finalizedAt))
        {
            throw new IllegalArgumentException("artifactExpiresAt must be after finalizedAt");
        }
    }

    private static String requireNonBlank(String value, String name)
    {
        Objects.requireNonNull(value, name + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(name + " must not be blank");
        }
        return value;
    }
}
