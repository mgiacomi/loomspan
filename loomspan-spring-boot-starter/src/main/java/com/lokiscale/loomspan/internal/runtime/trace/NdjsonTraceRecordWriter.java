package com.lokiscale.loomspan.internal.runtime.trace;

import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.core.TraceRecord;

import java.io.IOException;
import java.io.Writer;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.util.Objects;

final class NdjsonTraceRecordWriter implements TraceRecordWriter
{
    private final Path tracePath;
    private final ObjectMapper objectMapper;

    public NdjsonTraceRecordWriter(Path tracePath)
    {
        this(tracePath, com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().canonicalTrace());
    }

    NdjsonTraceRecordWriter(Path tracePath, ObjectMapper objectMapper)
    {
        this.tracePath = Objects.requireNonNull(tracePath, "tracePath must not be null");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
    }

    @Override
    public synchronized void append(TraceRecord record) throws IOException
    {
        Files.createDirectories(tracePath.getParent());

        try (Writer writer = Files.newBufferedWriter(
                tracePath,
                StandardCharsets.UTF_8,
                StandardOpenOption.CREATE,
                StandardOpenOption.WRITE,
                StandardOpenOption.APPEND))
        {
            writer.write(objectMapper.writeValueAsString(record));
            writer.write('\n');
        }
    }
}
