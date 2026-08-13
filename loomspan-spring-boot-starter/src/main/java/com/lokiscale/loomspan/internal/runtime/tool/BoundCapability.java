package com.lokiscale.loomspan.internal.runtime.tool;

import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import org.springframework.lang.Nullable;

import java.util.Map;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Objects;

public final class BoundCapability
{
    @FunctionalInterface
    public interface Invocation
    {
        Object invoke(Map<String, Object> arguments, @Nullable String linkedTaskId);
    }

    private final CapabilityMetadata metadata;
    private final Invocation invocation;

    public BoundCapability(CapabilityMetadata metadata, Invocation invocation)
    {
        this.metadata = Objects.requireNonNull(metadata, "metadata must not be null");
        this.invocation = Objects.requireNonNull(invocation, "invocation must not be null");
    }

    public CapabilityMetadata metadata() { return metadata; }
    public String name() { return metadata.name(); }
    public String description() { return metadata.tool().description(); }
    public String inputSchema() { return metadata.tool().inputSchema(); }

    public Object invoke(Map<String, Object> arguments, @Nullable String linkedTaskId)
    {
        Map<String, Object> copiedArguments = arguments == null
                ? Map.of()
                : Collections.unmodifiableMap(new LinkedHashMap<>(arguments));
        return invocation.invoke(copiedArguments, linkedTaskId);
    }
}
