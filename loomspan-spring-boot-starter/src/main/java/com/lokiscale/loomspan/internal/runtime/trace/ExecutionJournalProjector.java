package com.lokiscale.loomspan.internal.runtime.trace;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.core.ExecutionJournal;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.JournalLevel;
import com.lokiscale.loomspan.internal.core.TraceRecord;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public final class ExecutionJournalProjector
{
    private final ObjectMapper objectMapper;

    public ExecutionJournalProjector()
    {
        this(com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().canonicalTrace());
    }

    public ExecutionJournalProjector(ObjectMapper objectMapper)
    {
        this.objectMapper = java.util.Objects.requireNonNull(objectMapper, "objectMapper must not be null");
    }

    public ExecutionJournal project(List<TraceRecord> records)
    {
        // The journal is the single developer-facing projection we keep in the runtime.
        List<JournalEntry> entries = new ArrayList<>();
        TraceRecord previousRecord = null;

        for (TraceRecord record : records == null ? List.<TraceRecord>of() : records)
        {
            appendRecord(entries, previousRecord, record);
            previousRecord = record;
        }

        return new ExecutionJournal(entries);
    }

    public ExecutionJournal project(com.lokiscale.loomspan.internal.core.ExecutionTraceHandle traceHandle) throws java.io.IOException
    {
        List<JournalEntry> entries = new ArrayList<>();
        final TraceRecord[] previousRecord = new TraceRecord[1];

        traceHandle.readRecords(record ->
        {
            appendRecord(entries, previousRecord[0], record);
            previousRecord[0] = record;
        });

        return new ExecutionJournal(entries);
    }

    private void appendRecord(List<JournalEntry> entries, TraceRecord previousRecord, TraceRecord record)
    {
        JournalEntry entry = toJournalEntry(record);
        if (entry != null)
        {
            entries.add(entry);
        }
    }

    private JournalEntry toJournalEntry(TraceRecord record)
    {
        if (record == null)
        {
            return null;
        }

        return switch (record.recordType())
        {
            case MODEL_THOUGHT_CAPTURED -> entry(record, JournalLevel.INFO, JournalEntryType.THOUGHT, sanitize(record.data()));
            case PLAN_CREATED -> entry(record, JournalLevel.INFO, JournalEntryType.PLAN_CREATED, sanitize(record.data()));
            case PLAN_UPDATED -> entry(record, JournalLevel.INFO, JournalEntryType.PLAN_UPDATED, sanitize(record.data()));
            case LINTER_RECORDED -> entry(record, JournalLevel.INFO, JournalEntryType.LINTER, sanitize(record.data()));
            case STRUCTURED_OUTPUT_RECORDED -> entry(record, JournalLevel.INFO, JournalEntryType.OUTPUT_SCHEMA, sanitize(record.data()));
            case TOOL_CALL_STARTED -> entry(record, JournalLevel.INFO, toolCallType(record), summarizeToolCall(record));
            case TOOL_CALL_FAILED -> entry(record, JournalLevel.ERROR, JournalEntryType.TOOL_FAILURE, summarizeToolFailure(record));
            case TOOL_CALL_COMPLETED -> entry(record, JournalLevel.INFO, JournalEntryType.TOOL_RESULT, summarizeToolResult(record));
            case STEP_FAILED -> entry(record, JournalLevel.ERROR, JournalEntryType.STEP_FAILURE, summarizeError(record));
            case ERROR_RECORDED -> entry(record, JournalLevel.ERROR, JournalEntryType.ERROR, summarizeError(record));
            default -> null;
        };
    }

    private JournalEntryType toolCallType(TraceRecord record)
    {
        if (Boolean.TRUE.equals(record.metadata().get("unplanned")))
        {
            return JournalEntryType.UNPLANNED_TOOL_EXECUTION;
        }

        String linkedTaskId = linkedTask(record);
        if (linkedTaskId != null)
        {
            return JournalEntryType.TOOL_CALL;
        }

        JsonNode data = record.data();
        if (data != null && data.hasNonNull("linkedTaskId"))
        {
            return JournalEntryType.TOOL_CALL;
        }
        return JournalEntryType.TOOL_CALL;
    }

    private String linkedTask(TraceRecord record)
    {
        Object linkedTaskId = record.metadata().get("linkedTaskId");
        return linkedTaskId == null ? null : String.valueOf(linkedTaskId);
    }

    private JournalEntry entry(TraceRecord record, JournalLevel level, JournalEntryType type, Object payload)
    {
        return new JournalEntry(
                record.timestamp(),
                level,
                type,
                objectMapper.valueToTree(payload == null ? Map.of() : payload),
                record.frameId(),
                record.route());
    }

    private Object summarizeToolCall(TraceRecord record)
    {
        JsonNode data = record.data();
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();

        String capabilityName = firstNonBlank(
                metadataText(record, "capabilityName"),
                jsonText(data, "capabilityName"),
                jsonText(data, "route"),
                jsonText(data, "tool"),
                record.route());

        String linkedTaskId = firstNonBlank(linkedTask(record), jsonText(data, "linkedTaskId"));
        if (capabilityName != null)
        {
            payload.put("capabilityName", capabilityName);
        }
        if (linkedTaskId != null)
        {
            payload.put("linkedTaskId", linkedTaskId);
        }
        if (data != null && data.isObject())
        {
            JsonNode details = data.has("details") ? data.get("details") : data;
            payload.put("details", sanitize(details));
        }
        else
        {
            payload.put("details", Map.of("summary", "Tool call requested"));
        }

        return payload;
    }

    private Object summarizeToolResult(TraceRecord record)
    {
        JsonNode data = record.data();
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();

        String capabilityName = firstNonBlank(
                metadataText(record, "capabilityName"),
                jsonText(data, "capabilityName"),
                jsonText(data, "route"),
                jsonText(data, "tool"),
                record.route());

        String linkedTaskId = firstNonBlank(linkedTask(record), jsonText(data, "linkedTaskId"));
        if (capabilityName != null)
        {
            payload.put("capabilityName", capabilityName);
        }
        if (linkedTaskId != null)
        {
            payload.put("linkedTaskId", linkedTaskId);
        }
        if (data != null && data.isObject())
        {
            JsonNode details = data.has("details") ? data.get("details") : data;
            payload.put("details", sanitize(details));
        }
        else
        {
            payload.put("details", Map.of("summary", "Tool result recorded"));
        }
        return payload;
    }

    private Object summarizeError(TraceRecord record)
    {
        JsonNode data = record.data();
        if (data != null && data.isTextual())
        {
            return data;
        }

        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("sourceRecordType", record.recordType().name());

        String tool = firstNonBlank(
                metadataText(record, "capabilityName"),
                jsonText(data, "tool"),
                jsonText(data, "capabilityName"),
                jsonText(data, "route"));

        if (tool != null)
        {
            payload.put("tool", tool);
        }

        String linkedTaskId = firstNonBlank(linkedTask(record), jsonText(data, "linkedTaskId"));
        if (linkedTaskId != null)
        {
            payload.put("linkedTaskId", linkedTaskId);
        }

        payload.put("message", firstNonBlank(
                metadataText(record, "message"),
                jsonText(data, "message"),
                record.data() != null && record.data().isTextual() ? record.data().asText() : null,
                "Error recorded"));

        String exceptionType = firstNonBlank(metadataText(record, "exceptionType"), jsonText(data, "exceptionType"));
        if (exceptionType != null)
        {
            payload.put("exceptionType", exceptionType);
        }

        return payload;
    }

    private Object summarizeToolFailure(TraceRecord record)
    {
        JsonNode data = record.data();
        JsonNode failure = nestedObject(data, "failure");
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("sourceRecordType", record.recordType().name());

        String capabilityName = firstNonBlank(
                metadataText(record, "capabilityName"),
                jsonText(failure, "capabilityName"),
                jsonText(data, "tool"),
                jsonText(data, "capabilityName"),
                record.route());

        if (capabilityName != null)
        {
            payload.put("tool", capabilityName);
        }

        String linkedTaskId = firstNonBlank(
                linkedTask(record),
                jsonText(failure, "linkedTaskId"),
                jsonText(data, "linkedTaskId"));

        if (linkedTaskId != null)
        {
            payload.put("linkedTaskId", linkedTaskId);
        }

        payload.put("message", firstNonBlank(
                metadataText(record, "message"),
                jsonText(failure, "message"),
                jsonText(data, "message"),
                "Tool execution failed"));

        String exceptionType = firstNonBlank(
                metadataText(record, "exceptionType"),
                jsonText(failure, "exceptionType"),
                jsonText(data, "exceptionType"));

        if (exceptionType != null)
        {
            payload.put("exceptionType", exceptionType);
        }
        if (data != null && data.isObject())
        {
            JsonNode details = failure != null ? failure : data;
            payload.put("details", sanitize(details));
        }

        return payload;
    }

    private JsonNode sanitize(JsonNode node)
    {
        if (node == null)
        {
            return objectMapper.nullNode();
        }
        if (node.isObject())
        {
            var copy = objectMapper.createObjectNode();
            node.properties().iterator().forEachRemaining(entry -> copy.set(entry.getKey(), shouldRedact(entry.getKey())
                    ? objectMapper.getNodeFactory().textNode("[redacted]")
                    : sanitize(entry.getValue())));
            return copy;
        }
        if (node.isArray())
        {
            var copy = objectMapper.createArrayNode();
            node.forEach(value -> copy.add(sanitize(value)));
            return copy;
        }

        return node.deepCopy();
    }

    private boolean shouldRedact(String fieldName)
    {
        if (fieldName == null)
        {
            return false;
        }

        String normalized = fieldName.toLowerCase();

        return normalized.contains("secret")
                || normalized.contains("token")
                || normalized.contains("authorization")
                || normalized.contains("auth")
                || normalized.contains("credential")
                || normalized.contains("bearer")
                || normalized.contains("apikey")
                || normalized.contains("api_key")
                || normalized.contains("password");
    }

    private String metadataText(TraceRecord record, String fieldName)
    {
        Object value = record.metadata().get(fieldName);
        return value == null ? null : String.valueOf(value);
    }

    private String jsonText(JsonNode node, String fieldName)
    {
        if (node == null || !node.isObject())
        {
            return null;
        }

        JsonNode value = node.get(fieldName);
        if (value == null || value.isNull() || value.isContainer())
        {
            return null;
        }

        return value.asText();
    }

    private JsonNode nestedObject(JsonNode node, String fieldName)
    {
        if (node == null || !node.isObject())
        {
            return null;
        }

        JsonNode value = node.get(fieldName);
        return value != null && value.isObject() ? value : null;
    }

    private String firstNonBlank(String... values)
    {
        for (String value : values)
        {
            if (value != null && !value.isBlank())
            {
                return value;
            }
        }
        return null;
    }
}
