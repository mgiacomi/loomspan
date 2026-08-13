package com.lokiscale.loomspan.internal.core;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import tools.jackson.databind.JsonNode;
import org.springframework.lang.Nullable;

import java.time.Instant;
import java.util.Objects;

public record JournalEntry(
        Instant timestamp,
        JournalLevel level,
        JournalEntryType type,
        JsonNode payload,
        @Nullable String frameId,
        @Nullable String route)
{
    public JournalEntry(Instant timestamp,
            JournalLevel level,
            JournalEntryType type,
            JsonNode payload)
    {
        this(timestamp, level, type, payload, null, null);
    }

    @JsonCreator
    public JournalEntry(
            @JsonProperty("timestamp") Instant timestamp,
            @JsonProperty("level") JournalLevel level,
            @JsonProperty("type") JournalEntryType type,
            @JsonProperty("payload") JsonNode payload,
            @JsonProperty("frameId") @Nullable String frameId,
            @JsonProperty("route") @Nullable String route)
    {
        timestamp = Objects.requireNonNull(timestamp, "timestamp must not be null");
        level = Objects.requireNonNull(level, "level must not be null");
        type = Objects.requireNonNull(type, "type must not be null");
        payload = Objects.requireNonNull(payload, "payload must not be null");
        frameId = normalizeNullable(frameId);
        route = normalizeNullable(route);
        this.timestamp = timestamp;
        this.level = level;
        this.type = type;
        this.payload = payload;
        this.frameId = frameId;
        this.route = route;
    }

    private static String normalizeNullable(String value)
    {
        if (value == null || value.isBlank())
        {
            return null;
        }
        return value;
    }
}
