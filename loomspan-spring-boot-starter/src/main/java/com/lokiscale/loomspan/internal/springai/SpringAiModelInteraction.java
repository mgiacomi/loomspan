package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.model.ModelInteractionResult;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.util.MimeTypeUtils;

import java.util.Map;
import java.util.Objects;

public final class SpringAiModelInteraction implements ModelInteraction
{
    private final ChatClient chatClient;

    public SpringAiModelInteraction(ChatClient chatClient)
    {
        this.chatClient = Objects.requireNonNull(chatClient, "chatClient must not be null");
    }

    @Override
    public ModelInteractionResult call(ModelInteractionRequest interactionRequest)
    {
        try
        {
            return doCall(interactionRequest);
        }
        catch (org.springframework.ai.tool.execution.ToolExecutionException ex)
        {
            if (ex.getCause() instanceof RuntimeException runtimeException) throw runtimeException;
            throw ex;
        }
    }

    private ModelInteractionResult doCall(ModelInteractionRequest interactionRequest)
    {
        ChatClient.ChatClientRequestSpec request = chatClient.prompt().system(interactionRequest.systemPrompt());
        if (interactionRequest.input().attachments().isEmpty())
        {
            request = request.user(interactionRequest.input().userText());
        }
        else
        {
            request = request.user(user ->
            {
                user.text(interactionRequest.input().userText());
                interactionRequest.input().attachments().forEach(attachment -> user.media(
                        MimeTypeUtils.parseMimeType(attachment.contentType()), attachment.resource()));
            });
        }
        if (!interactionRequest.capabilities().isEmpty())
        {
            request = request.tools(SpringAiToolCallbackAdapter.adapt(interactionRequest.capabilities()));
        }
        ChatClientResponse response = request
                .advisors(spec ->
                {
                    spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, interactionRequest.traceContext());
                    if (interactionRequest.planning())
                    {
                        spec.param(com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor.PLANNING_CALL_KEY, true);
                    }
                })
                .call()
                .chatClientResponse();
        String content = response.chatResponse() == null || response.chatResponse().getResult() == null
                || response.chatResponse().getResult().getOutput() == null
                ? ""
                : response.chatResponse().getResult().getOutput().getText();
        Map<String, Object> context = response.context() == null ? Map.of() : response.context();
        return new ModelInteractionResult(content, context);
    }
}
