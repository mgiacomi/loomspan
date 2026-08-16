package com.lokiscale.loomspan.internal.runtime;

import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.model.ModelInteractionResult;
import org.springframework.core.io.Resource;
import org.springframework.util.MimeType;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import com.lokiscale.loomspan.internal.core.ModelTraceContext;

public class SimpleChatClient implements ModelInteraction {

    private static final ObjectMapper OBJECT_MAPPER = JsonMapper.builder()
            .findAndAddModules()
            .build();

    private final ExecutionPlan plan;
    private final String content;
    private final List<String> systemMessagesSeen = new ArrayList<>();
    private final List<String> userMessagesSeen = new ArrayList<>();
    private final List<CapturedMedia> userMediaSeen = new ArrayList<>();

    public SimpleChatClient(ExecutionPlan plan, String content) {
        this.plan = plan;
        this.content = content;
    }

    public List<String> getSystemMessagesSeen() {
        return systemMessagesSeen;
    }

    public List<String> getUserMessagesSeen() {
        return userMessagesSeen;
    }

    public List<CapturedMedia> getUserMediaSeen() {
        return userMediaSeen;
    }

    @Override
    public ModelInteractionResult call(ModelInteractionRequest request) {
        systemMessagesSeen.add(request.systemPrompt());
        userMessagesSeen.add(request.input().userText());
        request.input().attachments().forEach(attachment -> userMediaSeen.add(
                new CapturedMedia(MimeType.valueOf(attachment.contentType()), attachment.resource())));
        if (plan != null) {
            try {
                return withAttemptContext(request, OBJECT_MAPPER.writeValueAsString(plan));
            }
            catch (JacksonException ex) {
                throw new IllegalStateException("Failed to serialize execution plan", ex);
            }
        }
        return withAttemptContext(request, content);
    }

    private ModelInteractionResult withAttemptContext(ModelInteractionRequest request, String responseContent) {
        return new ModelInteractionResult(responseContent, Map.of(
                ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY,
                request.traceContext().nextAttempt()));
    }

    public record CapturedMedia(MimeType mimeType, Resource resource) {
    }
}
