package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import io.micrometer.observation.Observation;
import io.micrometer.observation.ObservationHandler;
import io.micrometer.observation.ObservationRegistry;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.openai.OpenAiChatOptions;
import org.springframework.ai.tool.function.FunctionToolCallback;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.core.io.DefaultResourceLoader;
import com.lokiscale.loomspan.internal.chat.ProviderAttemptCallAdvisor;
import com.lokiscale.loomspan.internal.core.*;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.*;
import com.lokiscale.loomspan.internal.provider.*;

import java.time.Clock;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SpringAiObservationIntegrationTest
{
    @Test
    void springAiObservationsAreSafeByDefault() throws Exception
    {
        String promptCanary = "prompt-secret-741";
        String completionCanary = "completion-secret-852";
        String keyCanary = "key-secret-963";
        String headerCanary = "header-secret-174";
        String baseUrlCanary = "base-secret-285";
        List<String> stopped = new ArrayList<>();
        ObservationRegistry registry = recordingRegistry(stopped);

        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                    {"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-test",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],
                     "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                    """.formatted(completionCanary)));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey(keyCanary);
            properties.setBaseUrl(server.url("/" + baseUrlCanary).toString());
            properties.setHeaders(Map.of("X-Observation-Canary", headerCanary));

            var model = new SpringAiProviderIntegration(new DefaultResourceLoader(), registry)
                    .create("openai-main", properties).chatModel();
            model.call(new Prompt(promptCanary, OpenAiChatOptions.builder().model("gpt-test").build()));
        }

        assertThat(stopped).isNotEmpty();
        assertThat(String.join("\n", stopped))
                .doesNotContain(promptCanary, completionCanary, keyCanary, headerCanary, baseUrlCanary);
    }

    @Test
    void toolArgumentsAndProviderFailureBodiesAreNotExportedAndAccountingRemainsExact() throws Exception
    {
        String toolArgumentCanary = "tool-argument-secret-396";
        String providerBodyCanary = "provider-body-secret-407";
        List<String> stopped = new ArrayList<>();
        ObservationRegistry registry = recordingRegistry(stopped);

        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse().setResponseCode(503).setHeader("Content-Type", "application/json")
                    .setBody("{\"error\":{\"message\":\"" + providerBodyCanary + "\"}}"));
            server.enqueue(toolCall(toolArgumentCanary));
            server.enqueue(text("complete"));

            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("retry-key-secret-518");
            properties.setBaseUrl(server.url("/v1").toString());
            properties.getProviderRetry().setEnabled(true);
            properties.getProviderRetry().setMaxAttempts(2);
            properties.getProviderRetry().setInitialBackoff(java.time.Duration.ZERO);
            properties.getProviderRetry().setMaxBackoff(java.time.Duration.ZERO);
            var providerRuntime = new SpringAiProviderIntegration(new DefaultResourceLoader(), registry)
                    .create("openai", properties);
            var runtime = new ProviderConnectionRuntime(providerRuntime.chatModel(), AiDriver.OPENAI,
                    AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                    ProviderRetryPolicy.from(properties.getProviderRetry()),
                    ignored -> new ProviderFailureDetails(ProviderFailureClassification.TRANSIENT,
                            ProviderFailureCategory.SERVER_ERROR, 503, null, null, null,
                            "Provider temporarily unavailable", List.of()));
            DefaultSessionUsageService usage = new DefaultSessionUsageService(
                    new LoomspanProperties().getSession().getQuotas(), new NoOpUsageMetricsRecorder());
            DefaultExecutionStateService state = new DefaultExecutionStateService(Clock.systemUTC(), usage);
            LoomspanSession session = TestLoomspanSessions.withId("observation-retry", "test.skill", 8);
            state.openMissionFrame(session, "test.skill", Map.of());
            state.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
            AtomicInteger toolExecutions = new AtomicInteger();
            var tool = FunctionToolCallback.<Map<String, Object>, String>builder("lookup", (arguments, context) -> {
                        toolExecutions.incrementAndGet();
                        assertThat(arguments).containsEntry("value", toolArgumentCanary);
                        return "safe-result";
                    })
                    .description("lookup")
                    .inputType(new ParameterizedTypeReference<Map<String, Object>>() { })
                    .inputSchema("{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}}}")
                    .build();
            ChatClient client = ChatClient.builder(runtime.chatModel(), registry, null, null)
                    .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, state, new ModelUsageExtractor(), usage))
                    .build();

            String result = SessionContextRunner.callWithSession(session, () -> client.prompt().user("safe-user")
                    .options(OpenAiChatOptions.builder().model("gpt-test"))
                    .tools(tool)
                    .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY,
                            new ModelTraceContext(new ModelExecutionIdentity("gpt-test", "openai", AiDriver.OPENAI,
                                    "gpt-test"), "test.skill", "mission")))
                    .call().content());

            assertThat(result).isEqualTo("complete");
            assertThat(server.getRequestCount()).isEqualTo(3);
            assertThat(toolExecutions).hasValue(1);
            assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(3);
            assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isEqualTo(2);
            assertThat(records(session, TraceRecordType.MODEL_REQUEST_SENT)).hasSize(3);
            assertThat(records(session, TraceRecordType.MODEL_ATTEMPT_FAILED)).hasSize(1);
            assertThat(records(session, TraceRecordType.MODEL_RESPONSE_RECEIVED)).hasSize(2);
        }

        assertThat(stopped).isNotEmpty();
        assertThat(String.join("\n", stopped))
                .doesNotContain(toolArgumentCanary, providerBodyCanary, "retry-key-secret-518");
    }

    @Test
    void terminalFailureObservationDoesNotExportProviderBodyOrDuplicateAttemptFacts() throws Exception
    {
        String bodyCanary = "terminal-provider-body-secret-629";
        List<String> stopped = new ArrayList<>();
        ObservationRegistry registry = recordingRegistry(stopped);
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse().setResponseCode(400).setHeader("Content-Type", "application/json")
                    .setBody("{\"error\":{\"message\":\"" + bodyCanary + "\"}}"));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("terminal-key-secret-730");
            properties.setBaseUrl(server.url("/v1").toString());
            properties.getProviderRetry().setEnabled(false);
            ProviderConnectionRuntime runtime = new SpringAiProviderIntegration(new DefaultResourceLoader(), registry)
                    .create("openai", properties);
            DefaultSessionUsageService usage = new DefaultSessionUsageService(
                    new LoomspanProperties().getSession().getQuotas(), new NoOpUsageMetricsRecorder());
            DefaultExecutionStateService state = new DefaultExecutionStateService(Clock.systemUTC(), usage);
            LoomspanSession session = TestLoomspanSessions.withId("observation-failure", "test.skill", 4);
            state.openMissionFrame(session, "test.skill", Map.of());
            state.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
            ChatClient client = ChatClient.builder(runtime.chatModel(), registry, null, null)
                    .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, state, new ModelUsageExtractor(), usage))
                    .build();

            assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt()
                    .user("safe-user")
                    .options(OpenAiChatOptions.builder().model("gpt-test"))
                    .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY,
                            new ModelTraceContext(new ModelExecutionIdentity("gpt-test", "openai", AiDriver.OPENAI,
                                    "gpt-test"), "test.skill", "mission")))
                    .call().content())).isInstanceOf(RuntimeException.class);

            assertThat(server.getRequestCount()).isOne();
            assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isOne();
            assertThat(records(session, TraceRecordType.MODEL_REQUEST_SENT)).hasSize(1);
            assertThat(records(session, TraceRecordType.MODEL_ATTEMPT_FAILED)).hasSize(1);
            assertThat(records(session, TraceRecordType.MODEL_RESPONSE_RECEIVED)).isEmpty();
        }
        assertThat(String.join("\n", stopped)).doesNotContain(bodyCanary, "terminal-key-secret-730");
    }

    private static ObservationRegistry recordingRegistry(List<String> stopped)
    {
        ObservationRegistry registry = ObservationRegistry.create();
        registry.observationConfig().observationHandler(new ObservationHandler<Observation.Context>()
        {
            @Override public void onStop(Observation.Context context)
            {
                stopped.add(context.getName() + " " + context.getAllKeyValues());
            }
            @Override public boolean supportsContext(Observation.Context context) { return true; }
        });
        return registry;
    }

    private static List<TraceRecord> records(LoomspanSession session, TraceRecordType type)
    {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(record -> { if (record.recordType() == type) records.add(record); });
        return records;
    }

    private static MockResponse toolCall(String value)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"tool","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":null,
                  "tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\\\"value\\\":\\\"%s\\\"}"}}]},"finish_reason":"tool_calls"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(value));
    }

    private static MockResponse text(String value)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"text","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(value));
    }
}
