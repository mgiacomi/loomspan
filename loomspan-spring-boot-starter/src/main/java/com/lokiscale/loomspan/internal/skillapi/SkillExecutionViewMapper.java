package com.lokiscale.loomspan.internal.skillapi;

import tools.jackson.core.type.TypeReference;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.api.SkillExecutionEvent;
import com.lokiscale.loomspan.api.SkillExecutionView;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.ExecutionJournal;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class SkillExecutionViewMapper
{
    private static final TypeReference<Map<String, Object>> MAP_TYPE = new TypeReference<>()
    {
    };
    private static final TypeReference<List<Object>> LIST_TYPE = new TypeReference<>()
    {
    };

    private final ObjectMapper objectMapper;

    SkillExecutionViewMapper(ObjectMapper objectMapper)
    {
        this.objectMapper = objectMapper;
    }

    SkillExecutionView map(LoomspanSession session)
    {
        return map(session.getSessionId(), session.getExecutionJournal());
    }

    SkillExecutionView map(String sessionId, ExecutionJournal journal)
    {
        List<SkillExecutionEvent> events = journal.getEntriesSnapshot().stream()
                .map(this::mapEvent)
                .toList();
        return new SkillExecutionView(sessionId, events);
    }

    private SkillExecutionEvent mapEvent(JournalEntry entry)
    {
        return new SkillExecutionEvent(
                entry.timestamp(),
                entry.level().name(),
                entry.type().name(),
                mapDetails(entry.payload()),
                entry.frameId(),
                entry.route());
    }

    private Map<String, Object> mapDetails(JsonNode payload)
    {
        if (payload == null || payload.isNull())
        {
            LinkedHashMap<String, Object> details = new LinkedHashMap<>();
            details.put("value", null);
            return details;
        }
        if (payload.isObject())
        {
            return objectMapper.convertValue(payload, MAP_TYPE);
        }

        LinkedHashMap<String, Object> details = new LinkedHashMap<>();
        if (payload.isTextual())
        {
            details.put("message", payload.asText());
        }
        else if (payload.isArray())
        {
            details.put("value", objectMapper.convertValue(payload, LIST_TYPE));
        }
        else
        {
            details.put("value", objectMapper.convertValue(payload, Object.class));
        }
        return details;
    }
}
