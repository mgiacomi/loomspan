package com.lokiscale.loomspan.internal.core;

import java.util.Map;
import java.util.Objects;

/**
 * Adds bounded failure details to loomspan-owned traces.
 */
public final class TraceFailureMetadata
{
    private TraceFailureMetadata()
    {
    }

    public static void addTo(Map<String, Object> metadata, Throwable failure, String safeMessage)
    {
        Objects.requireNonNull(metadata, "metadata must not be null");
        Objects.requireNonNull(failure, "failure must not be null");
        Objects.requireNonNull(safeMessage, "safeMessage must not be null");
        if (safeMessage.isBlank())
        {
            throw new IllegalArgumentException("safeMessage must not be blank");
        }
        metadata.put("exceptionType", failure.getClass().getName());
        metadata.put("message", safeMessage);
    }
}
