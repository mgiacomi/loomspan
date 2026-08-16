package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.model.ModelInteractionResult;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

class FakeCoordinatorChatClient implements ModelInteraction {

    private static final ObjectMapper OBJECT_MAPPER = JsonMapper.builder().findAndAddModules().build();

    private final ExecutionPlan plan;
    private final String content;
    private final String toolPayloadJson;
    private final boolean firstCallReturnsPlan;
    final List<String> toolNamesSeen = new ArrayList<>();
    final List<String> systemMessagesSeen = new ArrayList<>();
    final List<String> userMessagesSeen = new ArrayList<>();
    final List<List<String>> toolNamesByCall = new ArrayList<>();
    Object lastToolResult;
    private int callCount;

    FakeCoordinatorChatClient(ExecutionPlan plan, String content, String toolPayloadJson) {
        this(plan, content, toolPayloadJson, true);
    }

    FakeCoordinatorChatClient(ExecutionPlan plan, String content, String toolPayloadJson, boolean firstCallReturnsPlan) {
        this.plan = plan;
        this.content = content;
        this.toolPayloadJson = toolPayloadJson;
        this.firstCallReturnsPlan = firstCallReturnsPlan;
    }

    @Override
    public ModelInteractionResult call(ModelInteractionRequest request) {
        int current = ++callCount;
        systemMessagesSeen.add(request.systemPrompt());
        userMessagesSeen.add(request.input().userText());
        toolNamesSeen.clear();
        toolNamesSeen.addAll(request.capabilities().stream().map(BoundCapability::name).toList());
        toolNamesByCall.add(List.copyOf(toolNamesSeen));
        if ((!firstCallReturnsPlan && current == 1 || firstCallReturnsPlan && current == 2)
                && !request.capabilities().isEmpty()) {
            try {
                @SuppressWarnings("unchecked")
                Map<String, Object> arguments = OBJECT_MAPPER.readValue(toolPayloadJson, Map.class);
                lastToolResult = request.capabilities().getFirst().invoke(arguments, null);
            }
            catch (JacksonException ex) {
                throw new IllegalStateException(ex);
            }
        }
        Object payload = firstCallReturnsPlan && current == 1 ? plan : content;
        if (payload instanceof ExecutionPlan executionPlan) {
            try {
                return withAttemptContext(request, OBJECT_MAPPER.writeValueAsString(executionPlan));
            }
            catch (JacksonException ex) {
                throw new IllegalStateException(ex);
            }
        }
        return withAttemptContext(request, String.valueOf(payload));
    }

    private ModelInteractionResult withAttemptContext(ModelInteractionRequest request, String responseContent) {
        return new ModelInteractionResult(responseContent, Map.of(
                ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY,
                request.traceContext().nextAttempt()));
    }
}
