package com.lokiscale.loomspan.internal.runtime.trace;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.node.StringNode;
import com.lokiscale.loomspan.internal.core.ExecutionTraceReader;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;

import java.io.IOException;
import java.io.BufferedReader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.function.Consumer;

final class NdjsonExecutionTraceReader implements ExecutionTraceReader
{
    private final ObjectMapper objectMapper;

    NdjsonExecutionTraceReader()
    {
        this(com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().canonicalTrace());
    }

    NdjsonExecutionTraceReader(ObjectMapper objectMapper)
    {
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
    }

    @Override
    public void read(Path tracePath, Consumer<TraceRecord> consumer) throws IOException
    {
        Objects.requireNonNull(consumer, "consumer must not be null");
        if (tracePath == null || Files.notExists(tracePath))
        {
            return;
        }

        try (BufferedReader reader = Files.newBufferedReader(tracePath))
        {
            PendingChunkedRecord pendingChunkedRecord = null;
            StringBuilder lineBuffer = new StringBuilder();
            int nextChar;

            while ((nextChar = reader.read()) != -1)
            {
                char current = (char) nextChar;
                if (current == '\n' || current == '\r')
                {
                    if (current == '\r')
                    {
                        reader.mark(1);
                        int following = reader.read();
                        if (following != '\n' && following != -1)
                        {
                            reader.reset();
                        }
                    }
                    pendingChunkedRecord = processLine(lineBuffer.toString(), pendingChunkedRecord, consumer, false);
                    lineBuffer.setLength(0);
                    continue;
                }

                lineBuffer.append(current);
            }

            if (lineBuffer.length() > 0)
            {
                pendingChunkedRecord = processLine(lineBuffer.toString(), pendingChunkedRecord, consumer, true);
            }
            if (pendingChunkedRecord != null)
            {
                pendingChunkedRecord.emitPartialTo(consumer);
            }
        }
    }

    private PendingChunkedRecord processLine(String line,
            PendingChunkedRecord pendingChunkedRecord,
            Consumer<TraceRecord> consumer,
            boolean allowTrailingPartialRecord) throws IOException
    {
        if (line.isBlank())
        {
            return pendingChunkedRecord;
        }

        TraceRecord record;
        try
        {
            record = objectMapper.readValue(line, TraceRecord.class);
        }
        catch (JacksonException ex)
        {
            if (allowTrailingPartialRecord)
            {
                return pendingChunkedRecord;
            }
            throw new IOException(ex);
        }

        if (pendingChunkedRecord != null)
        {
            pendingChunkedRecord.append(record);
            if (pendingChunkedRecord.complete())
            {
                pendingChunkedRecord.emitTo(consumer);
                return null;
            }
            return pendingChunkedRecord;
        }
        if (Boolean.TRUE.equals(record.metadata().get("payloadChunked")))
        {
            return new PendingChunkedRecord(record);
        }

        consumer.accept(record);

        return null;
    }

    private JsonNode rebuildData(TraceRecord record, String content) throws IOException
    {
        Object contentType = record.metadata().get("contentType");
        if ("text/plain".equals(contentType))
        {
            return StringNode.valueOf(content);
        }
        return objectMapper.readTree(content);
    }

    private final class PendingChunkedRecord
    {

        private final TraceRecord envelope;
        private final int expectedChunkCount;
        private final Map<Integer, String> contentByChunkIndex;
        private final Map<Long, TraceRecord> chunksBySequence;

        private PendingChunkedRecord(TraceRecord envelope)
        {
            this.envelope = envelope;
            Object chunkCount = envelope.metadata().get("chunkCount");
            this.expectedChunkCount = chunkCount == null ? 0 : Integer.parseInt(String.valueOf(chunkCount));
            this.contentByChunkIndex = new LinkedHashMap<>();
            this.chunksBySequence = new LinkedHashMap<>();
        }

        private String payloadId()
        {
            return String.valueOf(envelope.metadata().get("payloadId"));
        }

        private void append(TraceRecord record) throws IOException
        {
            if (record.recordType() != TraceRecordType.PAYLOAD_CHUNK_APPENDED)
            {
                throw new IOException("Encountered record '" + record.recordType() + "' before completing chunked payload '" + payloadId() + "'");
            }

            Object payloadId = record.metadata().get("payloadId");
            if (!payloadId().equals(String.valueOf(payloadId)))
            {
                throw new IOException("Encountered chunk for payload '" + payloadId + "' while reconstructing '" + payloadId() + "'");
            }

            Object chunkIndexValue = record.metadata().get("chunkIndex");
            if (chunkIndexValue == null)
            {
                throw new IOException("Encountered chunk for payload '" + payloadId() + "' without a chunkIndex");
            }

            int chunkIndex = Integer.parseInt(String.valueOf(chunkIndexValue));
            chunksBySequence.put(record.sequence(), record);

            if (record.data() != null)
            {
                contentByChunkIndex.put(chunkIndex, record.data().asText(""));
            }
        }

        private boolean complete()
        {
            return expectedChunkCount > 0 && chunksBySequence.size() >= expectedChunkCount;
        }

        private void emitTo(Consumer<TraceRecord> consumer) throws IOException
        {
            consumer.accept(new TraceRecord(
                    envelope.traceId(),
                    envelope.sessionId(),
                    envelope.sequence(),
                    envelope.timestamp(),
                    envelope.recordType(),
                    envelope.frameId(),
                    envelope.parentFrameId(),
                    envelope.frameType(),
                    envelope.route(),
                    envelope.threadName(),
                    envelope.metadata(),
                    rebuildData(envelope, rebuildContent())));

            for (TraceRecord chunk : chunksBySequence.values())
            {
                consumer.accept(chunk);
            }
        }

        private void emitPartialTo(Consumer<TraceRecord> consumer)
        {
            consumer.accept(envelope);

            for (TraceRecord chunk : chunksBySequence.values())
            {
                consumer.accept(chunk);
            }
        }

        private String rebuildContent() throws IOException
        {
            StringBuilder content = new StringBuilder();

            for (int chunkIndex = 0; chunkIndex < expectedChunkCount; chunkIndex++)
            {
                String chunk = contentByChunkIndex.get(chunkIndex);
                if (chunk == null)
                {
                    throw new IOException("Missing chunk " + chunkIndex + " for payload '" + payloadId() + "'");
                }
                content.append(chunk);
            }

            return content.toString();
        }
    }
}
