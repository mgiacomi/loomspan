package com.lokiscale.loomspan.internal.observability.web;

import tools.jackson.databind.ObjectMapper;

import java.io.IOException;
import java.util.Objects;

public final class ObservabilityJsonCodec
{
    private final ObjectMapper mapper;

    public ObservabilityJsonCodec()
    {
        this(com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().strictObservability());
    }

    public ObservabilityJsonCodec(ObjectMapper mapper)
    {
        this.mapper = Objects.requireNonNull(mapper, "mapper must not be null");
    }

    byte[] write(Object value) throws IOException
    {
        return mapper.writeValueAsBytes(value);
    }

    <T> T read(byte[] bytes, Class<T> type) throws IOException
    {
        return mapper.readValue(bytes, type);
    }
}
