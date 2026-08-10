package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.provider.ProviderFailureDetails;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecider;
import com.lokiscale.loomspan.internal.provider.ProviderRetryOutcome;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.Message;
import org.springframework.core.Ordered;

import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.CancellationException;

final class ProviderAttemptCallAdvisor implements CallAdvisor
{
    static final int ORDER = Ordered.LOWEST_PRECEDENCE - 1;
    private final ProviderConnectionRuntime runtime;
    private final ExecutionStateService executionStateService;
    private final ModelUsageExtractor modelUsageExtractor;
    private final SessionUsageService sessionUsageService;
    private final ProviderRetryDecider retryDecider = new ProviderRetryDecider();

    ProviderAttemptCallAdvisor(ProviderConnectionRuntime runtime, ExecutionStateService executionStateService,
            ModelUsageExtractor modelUsageExtractor, SessionUsageService sessionUsageService)
    {
        this.runtime = Objects.requireNonNull(runtime, "runtime must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.modelUsageExtractor = Objects.requireNonNull(modelUsageExtractor, "modelUsageExtractor must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
    }

    @Override
    public ChatClientResponse adviseCall(ChatClientRequest request, CallAdvisorChain chain)
    {
        Objects.requireNonNull(request, "request must not be null");
        Objects.requireNonNull(chain, "chain must not be null");
        Object rawContext = request.context().get(ModelTraceContext.REQUEST_CONTEXT_KEY);
        if (!(rawContext instanceof ModelTraceContext context))
            throw new IllegalStateException("loomspan model call is missing its call-local trace context");

        LoomspanSession session = LoomspanSession.getCurrentSession();
        ExecutionFrame frame = session.peekFrame();
        Map<String, Object> requestPayload = requestPayload(request);
        for (int providerAttempt = 1; providerAttempt <= runtime.retryPolicy().maxAttempts(); providerAttempt++)
        {
            requireNotInterrupted();
            sessionUsageService.reserveProviderAttempt(session, context.skillName());
            Map<String, Object> attempt = context.nextAttempt(providerAttempt);
            executionStateService.recordModelRequestPrepared(session, frame, context, attempt, requestPayload);
            executionStateService.recordModelRequestSent(session, frame, context, attempt, requestPayload);

            ChatClientResponse response;
            try
            {
                response = chain.copy(this).nextCall(request);
            }
            catch (RuntimeException failure)
            {
                ProviderFailureDetails details = runtime.failureTranslator().translate(failure);
                ProviderRetryOutcome outcome = retryDecider.decide(runtime.retryPolicy(), details, providerAttempt);
                sessionUsageService.recordProviderAttemptOutcome(context.skillName(), context.identity(), "failed",
                        details.category(), outcome.decision());
                executionStateService.recordModelAttemptFailed(session, frame, context, attempt,
                        failureMetadata(details, outcome), failurePayload(details));
                if (outcome.decision() != com.lokiscale.loomspan.internal.provider.ProviderRetryDecision.RETRY)
                {
                    session.registerProviderFailure(failure, attempt);
                    throw failure;
                }
                waitInterruptibly(outcome.delay());
                continue;
            }

            ChatClientResponse linkedResponse = (response == null ? ChatClientResponse.builder() : response.mutate())
                    .context(ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, attempt).build();
            sessionUsageService.recordProviderAttemptOutcome(context.skillName(), context.identity(), "succeeded", null, null);
            String responseText = responseText(linkedResponse);
            ModelUsageRecord usage = modelUsageExtractor.extract(linkedResponse.chatResponse(),
                    request.prompt().getUserMessage().getText(), request.prompt().getSystemMessage().getText(), responseText);
            executionStateService.recordModelResponseReceived(session, frame, context, attempt, usage,
                    Map.of("content", responseText));
            sessionUsageService.recordModelResponse(session, context.skillName(), context.identity(), usage);
            return linkedResponse;
        }
        throw new IllegalStateException("provider retry loop exhausted without a terminal result");
    }

    private static Map<String, Object> failureMetadata(ProviderFailureDetails details, ProviderRetryOutcome outcome)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("failureClassification", details.classification().name());
        metadata.put("failureCategory", details.category().name());
        metadata.put("retryDecision", outcome.decision().name());
        metadata.put("retryDelayMillis", outcome.delay().toMillis());
        metadata.put("retryDelaySource", outcome.delaySource().name());
        if (details.httpStatus() != null) metadata.put("httpStatus", details.httpStatus());
        if (details.providerErrorType() != null) metadata.put("providerErrorType", details.providerErrorType());
        if (details.providerErrorCode() != null) metadata.put("providerErrorCode", details.providerErrorCode());
        if (details.summary() != null) metadata.put("summary", details.summary());
        return Map.copyOf(metadata);
    }

    private static Map<String, Object> failurePayload(ProviderFailureDetails details)
    {
        return details.diagnostics().isEmpty() ? Map.of() : Map.of("diagnostics", details.diagnostics());
    }

    private static void waitInterruptibly(Duration delay)
    {
        try
        {
            if (!delay.isZero()) Thread.sleep(delay.toMillis());
        }
        catch (InterruptedException ex)
        {
            Thread.currentThread().interrupt();
            throw new CancellationException("Provider retry interrupted");
        }
        requireNotInterrupted();
    }

    private static void requireNotInterrupted()
    {
        if (Thread.currentThread().isInterrupted()) throw new CancellationException("Provider retry interrupted");
    }

    @Override public String getName() { return "LoomspanProviderAttemptCallAdvisor"; }
    @Override public int getOrder() { return ORDER; }

    private Map<String, Object> requestPayload(ChatClientRequest request)
    {
        List<Map<String, Object>> messages = request.prompt().getInstructions().stream().map(this::messagePayload).toList();
        return Map.of("messages", messages);
    }

    private Map<String, Object> messagePayload(Message message)
    {
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("messageType", message.getMessageType().name());
        payload.put("text", message.getText() == null ? "" : message.getText());
        return Map.copyOf(payload);
    }

    private String responseText(ChatClientResponse response)
    {
        if (response.chatResponse() == null || response.chatResponse().getResult() == null
                || response.chatResponse().getResult().getOutput() == null
                || response.chatResponse().getResult().getOutput().getText() == null) return "";
        return response.chatResponse().getResult().getOutput().getText();
    }
}
