package com.lokiscale.loomspan.internal.runtime.trace;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.json.JsonMapper;
import com.fasterxml.jackson.databind.node.TextNode;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionTrace;
import com.lokiscale.loomspan.internal.core.ExecutionTraceHandle;
import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceCompletion;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.release.LoomspanReleaseVersion;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle;
import com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandle;
import org.springframework.lang.Nullable;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Collections;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.Consumer;
import java.util.function.Supplier;

public final class DefaultExecutionTraceHandle implements ExecutionTraceHandle
{
    private static final ObjectMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    private static final int DEFAULT_CHUNK_SIZE = 4096;

    private final String traceId;
    private final String sessionId;
    private final String entrySkill;
    private final Path tracePath;
    private final TracePersistencePolicy persistencePolicy;
    private final Clock clock;
    private final TraceRecordWriter writer;
    private final NdjsonExecutionTraceReader reader;
    private final AtomicLong sequence;
    private final AtomicBoolean initialized;
    private final Supplier<String> idSupplier;
    private final String threadName;
    private final String tracePathMetadata;
    private final ExecutionObservationHandle observationHandle;
    private final CompletionGraceRetention completionGraceRetention;
    private final ConfiguredLimitsSnapshot configuredLimits;

    private volatile boolean errored;
    private volatile boolean completed;
    private Optional<FinalizedTraceArtifact> finalizedArtifact = Optional.empty();

    public DefaultExecutionTraceHandle(String sessionId, String entrySkill, TracePersistencePolicy persistencePolicy, Clock clock)
    {
        this(newTraceId(), sessionId, entrySkill, null, persistencePolicy, false, false, clock, 0L, false,
                DefaultExecutionTraceHandle::newTraceId, null, null, NoOpExecutionObservationHandle.INSTANCE, null);
        resetTraceFile();
        initialize();
    }

    public DefaultExecutionTraceHandle(
            String sessionId,
            String entrySkill,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandle observationHandle)
    {
        this(newTraceId(), sessionId, entrySkill, null, persistencePolicy, false, false, clock, 0L, false,
                DefaultExecutionTraceHandle::newTraceId, null, null, observationHandle, null);
        resetTraceFile();
        initialize();
    }

    DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            Supplier<String> idSupplier,
            String threadName,
            String tracePathMetadata)
    {
        this(traceId, sessionId, entrySkill, tracePath, persistencePolicy, false, false, clock, 0L, false,
                idSupplier, threadName, tracePathMetadata, NoOpExecutionObservationHandle.INSTANCE, null);
        resetTraceFile();
        initialize();
    }

    DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            Supplier<String> idSupplier,
            String threadName,
            String tracePathMetadata,
            ConfiguredLimitsSnapshot configuredLimits)
    {
        this(traceId, sessionId, entrySkill, tracePath, persistencePolicy, false, false, clock, 0L, false,
                idSupplier, threadName, tracePathMetadata, NoOpExecutionObservationHandle.INSTANCE,
                Objects.requireNonNull(configuredLimits, "configuredLimits must not be null"));
        resetTraceFile();
        initialize();
    }

    DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            Supplier<String> idSupplier,
            String threadName,
            String tracePathMetadata,
            ExecutionObservationHandle observationHandle)
    {
        this(traceId, sessionId, entrySkill, tracePath, persistencePolicy, false, false, clock, 0L, false,
                idSupplier, threadName, tracePathMetadata, observationHandle, null);
        resetTraceFile();
        initialize();
    }

    DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            Supplier<String> idSupplier,
            String threadName,
            String tracePathMetadata,
            ExecutionObservationHandle observationHandle,
            TraceRecordWriter writer)
    {
        this(traceId, sessionId, entrySkill, tracePath, persistencePolicy, false, false, clock, 0L, false,
                idSupplier, threadName, tracePathMetadata, observationHandle, null, writer,
                ImmediateCompletionRetention.INSTANCE);
        resetTraceFile();
        initialize();
    }

    private DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            boolean errored,
            boolean completed,
            Clock clock,
            long startingSequence,
            boolean initialized,
            Supplier<String> idSupplier,
            @Nullable String threadName,
            @Nullable String tracePathMetadata,
            ExecutionObservationHandle observationHandle,
            @Nullable ConfiguredLimitsSnapshot configuredLimits)
    {
        this(traceId, sessionId, entrySkill, tracePath, persistencePolicy, errored, completed, clock, startingSequence,
                initialized, idSupplier, threadName, tracePathMetadata, observationHandle, configuredLimits, null,
                ImmediateCompletionRetention.INSTANCE);
    }

    private DefaultExecutionTraceHandle(
            String traceId,
            String sessionId,
            String entrySkill,
            Path tracePath,
            TracePersistencePolicy persistencePolicy,
            boolean errored,
            boolean completed,
            Clock clock,
            long startingSequence,
            boolean initialized,
            Supplier<String> idSupplier,
            @Nullable String threadName,
            @Nullable String tracePathMetadata,
            ExecutionObservationHandle observationHandle,
            @Nullable ConfiguredLimitsSnapshot configuredLimits,
            @Nullable TraceRecordWriter writer,
            CompletionGraceRetention completionGraceRetention)
    {
        this.traceId = requireNonBlank(traceId, "traceId");
        this.sessionId = requireNonBlank(sessionId, "sessionId");
        this.entrySkill = requireNonBlank(entrySkill, "entrySkill");
        this.tracePath = tracePath == null ? defaultPath(this.sessionId, this.traceId) : tracePath;
        this.persistencePolicy = persistencePolicy == null ? TracePersistencePolicy.NEVER : persistencePolicy;
        this.errored = errored;
        this.completed = completed;
        this.clock = Objects.requireNonNull(clock, "clock must not be null");
        this.writer = writer == null ? new NdjsonTraceRecordWriter(this.tracePath) : writer;
        this.reader = new NdjsonExecutionTraceReader();
        this.sequence = new AtomicLong(startingSequence);
        this.initialized = new AtomicBoolean(initialized);
        this.idSupplier = Objects.requireNonNull(idSupplier, "idSupplier must not be null");
        this.threadName = threadName == null || threadName.isBlank() ? null : threadName;
        this.tracePathMetadata = tracePathMetadata == null ? this.tracePath.toString() : tracePathMetadata;
        this.observationHandle = Objects.requireNonNull(observationHandle, "observationHandle must not be null");
        this.configuredLimits = configuredLimits;
        this.completionGraceRetention = Objects.requireNonNull(
                completionGraceRetention, "completionGraceRetention must not be null");
    }

    public DefaultExecutionTraceHandle(
            String sessionId,
            String entrySkill,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandle observationHandle,
            CompletionGraceRetention completionGraceRetention)
    {
        this(newTraceId(), sessionId, entrySkill, null, persistencePolicy, false, false, clock, 0L, false,
                DefaultExecutionTraceHandle::newTraceId, null, null, observationHandle, null, null,
                completionGraceRetention);
        resetTraceFile();
        initialize();
    }

    public DefaultExecutionTraceHandle(
            String sessionId,
            String entrySkill,
            TracePersistencePolicy persistencePolicy,
            Clock clock,
            ExecutionObservationHandle observationHandle,
            CompletionGraceRetention completionGraceRetention,
            ConfiguredLimitsSnapshot configuredLimits)
    {
        this(newTraceId(), sessionId, entrySkill, null, persistencePolicy, false, false, clock, 0L, false,
                DefaultExecutionTraceHandle::newTraceId, null, null, observationHandle,
                Objects.requireNonNull(configuredLimits, "configuredLimits must not be null"), null,
                completionGraceRetention);
        resetTraceFile();
        initialize();
    }

    private void initialize()
    {
        try
        {
            if (initialized.compareAndSet(false, true))
            {
                LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
                metadata.put("tracePath", tracePathMetadata);
                metadata.put("consoleCompatibilityVersion", LoomspanReleaseVersion.load());
                if (configuredLimits != null)
                {
                    metadata.put("configuredLimits", configuredLimits.asMetadata());
                }
                appendInternal(TraceRecordType.TRACE_STARTED, null, null, null, null,
                        Collections.unmodifiableMap(metadata), Map.of("sessionId", sessionId));
                appendInternal(TraceRecordType.TRACE_CAPTURE_POLICY_RECORDED, null, null, null, null, Map.of("persistencePolicy", persistencePolicy.name()), null);
            }
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to initialize execution trace for session '" + sessionId + "'", ex);
        }
    }

    @Override
    public synchronized TraceRecord append(
            TraceRecordType recordType,
            ExecutionFrame frame,
            TraceFrameType frameType,
            Map<String, Object> metadata,
            Object data) throws IOException
    {
        Objects.requireNonNull(frame, "frame must not be null");
        return appendInternal(
                recordType,
                frame.frameId(),
                frame.parentFrameId(),
                frameType,
                frame.route(),
                metadata,
                data);
    }

    @Override
    public synchronized TraceRecord append(TraceRecordType recordType, Map<String, Object> metadata, Object data) throws IOException
    {
        return appendInternal(recordType, null, null, null, null, metadata, data);
    }

    @Override
    public ExecutionTrace snapshot()
    {
        return new ExecutionTrace(traceId, sessionId, visibleTracePath(), persistencePolicy, errored, completed);
    }

    @Override
    public Path tracePath()
    {
        return tracePath;
    }

    @Override
    public synchronized void markErrored()
    {
        errored = true;
    }

    @Override
    public synchronized Optional<FinalizedTraceArtifact> finalizeTrace(TraceCompletion completion) throws IOException
    {
        Objects.requireNonNull(completion, "completion must not be null");
        if (completed)
        {
            return finalizedArtifact;
        }

        initialize();
        Map<String, Object> metadata = new LinkedHashMap<>(completion.metadata());
        metadata.put("errored", errored);
        metadata.put("persistencePolicy", persistencePolicy.name());
        TraceRecord completedRecord = append(TraceRecordType.TRACE_COMPLETED, metadata, null);
        completed = true;

        if (shouldDeleteAfterCompletion())
        {
            Optional<CompletionGraceRetention.RetainedArtifact> retained =
                    completionGraceRetention.retainOrDelete(
                            tracePath, completedRecord.timestamp(), traceId, sessionId);
            if (retained.isEmpty())
            {
                finalizedArtifact = Optional.empty();
                return finalizedArtifact;
            }
            CompletionGraceRetention.RetainedArtifact retainedArtifact = retained.orElseThrow();
            finalizedArtifact = Optional.of(descriptor(
                    completion,
                    completedRecord.timestamp(),
                    retainedArtifact.expiresAt(),
                    retainedArtifact.sizeBytes()));
            return finalizedArtifact;
        }
        finalizedArtifact = Optional.of(descriptor(
                completion,
                completedRecord.timestamp(),
                null,
                Files.size(tracePath)));
        return finalizedArtifact;
    }

    private FinalizedTraceArtifact descriptor(
            TraceCompletion completion,
            Instant finalizedAt,
            @Nullable Instant artifactExpiresAt,
            long sizeBytes)
    {
        return new FinalizedTraceArtifact(
                traceId,
                sessionId,
                entrySkill,
                completion.outcome(),
                finalizedAt,
                tracePath,
                sizeBytes,
                persistencePolicy,
                artifactExpiresAt);
    }

    @Override
    public synchronized void readRecords(Consumer<TraceRecord> consumer) throws IOException
    {
        reader.read(tracePath, consumer);
    }

    private void resetTraceFile()
    {
        try
        {
            Files.deleteIfExists(tracePath);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to reset execution trace file for session '" + sessionId + "'", ex);
        }
    }

    private TraceRecord appendInternal(
            TraceRecordType recordType,
            @Nullable String frameId,
            @Nullable String parentFrameId,
            @Nullable TraceFrameType frameType,
            @Nullable String route,
            Map<String, Object> metadata,
            Object data) throws IOException
    {
        if (completed)
        {
            throw new IllegalStateException("Execution trace '" + traceId + "' is already completed");
        }

        initialize();
        JsonNode jsonData = toJson(data);
        Map<String, Object> safeMetadata = metadata == null ? Map.of() : new LinkedHashMap<>(metadata);
        long nextSequence = sequence.incrementAndGet();

        if (jsonData != null)
        {
            String serialized = jsonData.isTextual() ? jsonData.asText() : OBJECT_MAPPER.writeValueAsString(jsonData);
            if (serialized.length() > DEFAULT_CHUNK_SIZE)
            {
                String payloadId = requireNonBlank(idSupplier.get(), "payloadId");
                int chunkCount = (int) Math.ceil((double) serialized.length() / DEFAULT_CHUNK_SIZE);
                safeMetadata.put("payloadId", payloadId);
                safeMetadata.put("chunkCount", chunkCount);
                safeMetadata.put("payloadChunked", true);
                safeMetadata.put("contentType", jsonData.isTextual() ? "text/plain" : "application/json");
                TraceRecord envelope = buildRecord(
                        nextSequence,
                        recordType,
                        frameId,
                        parentFrameId,
                        frameType,
                        route,
                        safeMetadata,
                        null);
                TraceRecord logicalRecord = new TraceRecord(
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
                        jsonData);

                writer.append(envelope);
                writeChunks(payloadId, chunkCount, serialized, frameId, parentFrameId, frameType, route, safeMetadata);

                publish(logicalRecord);
                return envelope;
            }
        }

        TraceRecord record = buildRecord(nextSequence, recordType, frameId, parentFrameId, frameType, route, safeMetadata, jsonData);
        writer.append(record);
        publish(record);
        return record;
    }

    private void publish(TraceRecord logicalRecord)
    {
        try
        {
            observationHandle.recordAppended(logicalRecord);
        }
        catch (RuntimeException ignored)
        {
            // Optional observation must never change canonical trace behavior.
        }
    }

    private void writeChunks(
            String payloadId,
            int chunkCount,
            String serialized,
            @Nullable String frameId,
            @Nullable String parentFrameId,
            @Nullable TraceFrameType frameType,
            @Nullable String route,
            Map<String, Object> baseMetadata) throws IOException
    {
        for (int chunkIndex = 0; chunkIndex < chunkCount; chunkIndex++)
        {
            int start = chunkIndex * DEFAULT_CHUNK_SIZE;
            int end = Math.min(serialized.length(), start + DEFAULT_CHUNK_SIZE);
            Map<String, Object> metadata = new LinkedHashMap<>();
            metadata.put("payloadId", payloadId);
            metadata.put("chunkIndex", chunkIndex);
            metadata.put("chunkCount", chunkCount);
            metadata.put("contentType", baseMetadata.get("contentType"));

            TraceRecord chunk = buildRecord(
                    sequence.incrementAndGet(),
                    TraceRecordType.PAYLOAD_CHUNK_APPENDED,
                    frameId,
                    parentFrameId,
                    frameType,
                    route,
                    metadata,
                    TextNode.valueOf(serialized.substring(start, end)));

            writer.append(chunk);
        }
    }

    private TraceRecord buildRecord(
            long sequenceNumber,
            TraceRecordType recordType,
            @Nullable String frameId,
            @Nullable String parentFrameId,
            @Nullable TraceFrameType frameType,
            @Nullable String route,
            Map<String, Object> metadata,
            @Nullable JsonNode data)
    {
        return new TraceRecord(
                traceId,
                sessionId,
                sequenceNumber,
                resolveTimestamp(metadata),
                recordType,
                frameId,
                parentFrameId,
                frameType,
                route,
                threadName == null ? Thread.currentThread().getName() : threadName,
                metadata,
                data);
    }

    private Instant resolveTimestamp(Map<String, Object> metadata)
    {
        Object timestampOverride = metadata == null ? null : metadata.get("timestampOverride");
        if (timestampOverride instanceof String timestampText && !timestampText.isBlank())
        {
            try
            {
                return Instant.parse(timestampText);
            }
            catch (RuntimeException ignored)
            {
                // Fall back to append time if the override is malformed.
            }
        }
        return Instant.now(clock);
    }

    @Nullable
    private JsonNode toJson(Object data)
    {
        if (data == null)
        {
            return null;
        }
        if (data instanceof JsonNode jsonNode)
        {
            return jsonNode.deepCopy();
        }
        return OBJECT_MAPPER.valueToTree(data);
    }

    private boolean shouldDeleteAfterCompletion()
    {
        return switch (persistencePolicy)
        {
            case NEVER -> true;
            case ONERROR -> !errored;
            case ALWAYS -> false;
        };
    }

    @Nullable
    private String visibleTracePath()
    {
        if (completed && shouldDeleteAfterCompletion() && Files.notExists(tracePath))
        {
            return null;
        }
        return tracePath.toString();
    }

    private static Path defaultPath(String sessionId, String traceId)
    {
        return Path.of(System.getProperty("java.io.tmpdir"), sessionId + "." + traceId + ".execution-trace.ndjson");
    }

    private static String newTraceId()
    {
        return UUID.randomUUID().toString();
    }

    private static String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }
}
