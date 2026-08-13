package com.lokiscale.loomspan.internal.autoconfigure;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration;
import com.lokiscale.loomspan.internal.springai.SpringAiChatOptionsContributor;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderFailureClassification;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import org.springframework.core.io.DefaultResourceLoader;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.springframework.ai.anthropic.AnthropicChatOptions;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.ai.ollama.api.OllamaChatOptions;
import org.springframework.ai.openai.OpenAiChatOptions;

import java.util.Map;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchThrowable;

class ConnectionProtocolTest {

    @Test
    void explicitOpenRouterProfileRejectsPartialErrorCompletionsBeforeDecoding() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"id":"chatcmpl-error","object":"chat.completion","created":1,"model":"routed-model",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"unsafe partial content"},
                       "finish_reason":"error","error":{"message":"upstream overloaded","code":"E_OVERLOAD",
                       "metadata":{"error_type":"provider_overloaded"}}}]}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("gateway-key");
            properties.setBaseUrl(server.url("/api/v1").toString());
            LoomspanProperties.OpenAiOptions openAi = new LoomspanProperties.OpenAiOptions();
            openAi.setCompatibilityProfile(LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER);
            properties.setOpenai(openAi);
            var runtime = integration().create("openrouter", properties);

            Throwable failure = catchThrowable(() -> runtime.chatModel().call(
                    new Prompt("hello", OpenAiChatOptions.builder().model("routed-model").build())));
            var details = runtime.failureTranslator().translate(failure);

            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(details.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
            assertThat(details.category()).isEqualTo(ProviderFailureCategory.PROVIDER_OVERLOADED);
            assertThat(details.providerErrorType()).isEqualTo("provider_overloaded");
            assertThat(details.providerErrorCode()).isEqualTo("E_OVERLOAD");
            assertThat(details.diagnostics()).singleElement().satisfies(diagnostic ->
                    assertThat(diagnostic.get("text")).asString().contains("unsafe partial content"));
        }
    }

    @Test
    void openAiCompatibleBaseUrlEndingInV1DoesNotDuplicateTheVersionPath() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"routed-model",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
                     "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("gateway-key");
            properties.setBaseUrl(server.url("/api/v1").toString());

            var model = integration().create("gateway", properties).chatModel();
            model.call(new Prompt("hello", OpenAiChatOptions.builder().model("routed-model").build()));

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(request.getPath()).isEqualTo("/api/v1/chat/completions");
        }
    }

    @Test
    void anthropicConnectionUsesOfficialPathAndCommonStaticHeaders() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
                     "content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,
                     "usage":{"input_tokens":1,"output_tokens":1}}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.ANTHROPIC);
            properties.setApiKey("anthropic-secret");
            properties.setBaseUrl(server.url("/").toString());
            properties.setHeaders(Map.of("anthropic-beta", "test-beta"));

            var model = integration().create("anthropic-main", properties).chatModel();
            model.call(new Prompt("hello", AnthropicChatOptions.builder().model("claude-test").maxTokens(16).build()));

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(request.getPath()).isEqualTo("/v1/messages");
            assertThat(request.getHeader("x-api-key")).isEqualTo("anthropic-secret");
            assertThat(request.getHeader("anthropic-version")).isNotBlank();
            assertThat(request.getHeader("anthropic-beta")).isEqualTo("test-beta");
            assertThat(request.getBody().readUtf8()).contains("\"model\":\"claude-test\"");
        }
    }

    @ParameterizedTest
    @CsvSource({
            "low, 1024, 4096",
            "medium, 4096, 8192",
            "high, 8192, 16384"
    })
    void anthropicThinkingLevelsProduceProviderValidRequestTokenLimits(
            String level, long expectedBudget, int expectedMaxTokens) throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
                     "content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,
                     "usage":{"input_tokens":1,"output_tokens":1}}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.ANTHROPIC);
            properties.setApiKey("anthropic-secret");
            properties.setBaseUrl(server.url("/").toString());
            var options = new SpringAiChatOptionsContributor().createOptions(
                    new EffectiveSkillExecutionConfiguration(
                            "alias", "anthropic-main", AiDriver.ANTHROPIC, "claude-test", level))
                    .build();

            integration().create("anthropic-main", properties).chatModel().call(new Prompt("hello", options));

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            tools.jackson.databind.JsonNode body = tools.jackson.databind.json.JsonMapper.builder().build()
                    .readTree(request.getBody().readUtf8());
            assertThat(body.path("max_tokens").asInt()).isEqualTo(expectedMaxTokens);
            assertThat(body.path("thinking").path("type").asText()).isEqualTo("enabled");
            assertThat(body.path("thinking").path("budget_tokens").asLong()).isEqualTo(expectedBudget);
            assertThat(body.path("thinking").path("budget_tokens").asLong())
                    .isLessThan(body.path("max_tokens").asLong());
        }
    }

    @Test
    void anthropicHttpFailureIsCapturedOnceAndClassifiedForLoomspanRetry() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(new MockResponse().setResponseCode(503)
                    .setHeader("Content-Type", "application/json")
                    .setHeader("Retry-After", "2")
                    .setBody("{\"error\":{\"message\":\"temporarily unavailable\"}}"));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.ANTHROPIC);
            properties.setApiKey("anthropic-secret");
            properties.setBaseUrl(server.url("/").toString());
            var runtime = integration().create("anthropic-main", properties);

            Throwable failure = catchThrowable(() -> runtime.chatModel().call(new Prompt("hello",
                    AnthropicChatOptions.builder().model("claude-test").maxTokens(16).build())));
            var details = runtime.failureTranslator().translate(failure);

            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(details.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
            assertThat(details.category()).isEqualTo(ProviderFailureCategory.SERVER_ERROR);
            assertThat(details.httpStatus()).isEqualTo(503);
            assertThat(details.retryAfter()).isEqualTo(java.time.Duration.ofSeconds(2));
            assertThat(details.diagnostics()).singleElement().satisfies(diagnostic ->
                    assertThat(diagnostic.get("text")).asString().contains("temporarily unavailable"));
        }
    }

    @Test
    void openAiConnectionUsesConfiguredPathCredentialsAndHeaders() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-test",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
                     "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("secret-key");
            properties.setBaseUrl(server.url("/").toString());
            properties.setHeaders(Map.of("X-Tenant", "tenant-a"));
            LoomspanProperties.OpenAiOptions openAi = new LoomspanProperties.OpenAiOptions();
            openAi.setOrganizationId("org-a");
            openAi.setProjectId("project-a");
            properties.setOpenai(openAi);

            var model = integration().create("openai-main", properties).chatModel();
            model.call(new Prompt("hello", OpenAiChatOptions.builder().model("gpt-test").build()));

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(request.getPath()).isEqualTo("/chat/completions");
            assertThat(request.getHeader("Authorization")).isEqualTo("Bearer secret-key");
            assertThat(request.getHeader("X-Tenant")).isEqualTo("tenant-a");
            assertThat(request.getHeader("OpenAI-Organization")).isEqualTo("org-a");
            assertThat(request.getHeader("OpenAI-Project")).isEqualTo("project-a");
            assertThat(request.getBody().readUtf8()).contains("\"model\":\"gpt-test\"");
        }
    }

    @Test
    void ollamaConnectionUsesNativeChatEndpoint() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(json("""
                    {"model":"qwen","created_at":"2026-07-14T00:00:00Z",
                     "message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop",
                     "prompt_eval_count":1,"eval_count":1}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OLLAMA);
            properties.setBaseUrl(server.url("/").toString());

            var model = integration().create("ollama-local", properties).chatModel();
            model.call(new Prompt("hello", OllamaChatOptions.builder().model("qwen").build()));

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(request.getPath()).isEqualTo("/api/chat");
            assertThat(request.getBody().readUtf8()).contains("\"model\":\"qwen\"");
        }
    }

    @Test
    void ollamaRateLimitIsCapturedOnceAndClassifiedForLoomspanRetry() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            server.enqueue(new MockResponse().setResponseCode(429)
                    .setHeader("Content-Type", "application/json")
                    .setBody("{\"error\":\"busy\"}"));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OLLAMA);
            properties.setBaseUrl(server.url("/").toString());
            var runtime = integration().create("ollama-local", properties);

            Throwable failure = catchThrowable(() -> runtime.chatModel().call(
                    new Prompt("hello", OllamaChatOptions.builder().model("qwen").build())));
            var details = runtime.failureTranslator().translate(failure);

            assertThat(server.getRequestCount()).isEqualTo(1);
            assertThat(details.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
            assertThat(details.category()).isEqualTo(ProviderFailureCategory.RATE_LIMITED);
            assertThat(details.httpStatus()).isEqualTo(429);
        }
    }

    private static MockResponse json(String body) {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody(body);
    }

    private static SpringAiProviderIntegration integration() {
        return new SpringAiProviderIntegration(new DefaultResourceLoader());
    }
}
