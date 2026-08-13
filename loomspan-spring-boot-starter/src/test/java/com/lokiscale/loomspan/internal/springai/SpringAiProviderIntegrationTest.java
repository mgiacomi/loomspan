package com.lokiscale.loomspan.internal.springai;

import com.google.genai.errors.ClientException;
import com.google.genai.errors.GenAiIOException;
import com.google.genai.errors.ServerException;
import com.google.genai.Client;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderFailureClassification;
import org.junit.jupiter.api.Test;
import org.springframework.core.io.DefaultResourceLoader;
import org.springframework.ai.google.genai.GoogleGenAiChatOptions;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.ai.openai.OpenAiChatOptions;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.HttpStatusCode;
import org.springframework.http.client.ClientHttpResponse;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.MediaType;
import okhttp3.Protocol;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.ResponseBody;
import okio.BufferedSource;
import okio.Okio;

import java.io.IOException;
import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.net.SocketException;
import java.util.Optional;
import javax.net.ssl.SSLException;

import static org.assertj.core.api.Assertions.catchThrowable;

import static org.assertj.core.api.Assertions.assertThat;

class SpringAiProviderIntegrationTest
{
    private final SpringAiProviderIntegration integration =
            new SpringAiProviderIntegration(new DefaultResourceLoader());

    @Test
    void googleSdkRetriesAreDisabledAtTheHttpClientBoundary()
    {
        assertThat(SpringAiProviderIntegration.oneAttemptGoogleHttpOptions()
                .retryOptions().orElseThrow().attempts()).contains(1);
    }

    @Test
    void googleDirectInvocationMakesOneHttpAttemptOnRetryableFailure() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse().setResponseCode(503)
                    .setHeader("Content-Type", "application/json")
                    .setBody("{\"error\":{\"message\":\"temporarily unavailable\"}}"));
            Client.setDefaultBaseUrls(Optional.of(server.url("/").toString()), Optional.empty());
            try
            {
                var runtime = integration.create("gemini", gemini());
                Throwable failure = catchThrowable(() -> runtime.chatModel().call(new Prompt("hello",
                        GoogleGenAiChatOptions.builder().model("gemini-test").build())));

                assertThat(failure).isNotNull();
                assertThat(server.getRequestCount()).isEqualTo(1);
                assertThat(runtime.failureTranslator().translate(failure).classification())
                        .isEqualTo(ProviderFailureClassification.TRANSIENT);
            }
            finally
            {
                Client.setDefaultBaseUrls(Optional.empty(), Optional.empty());
            }
        }
    }

    @Test
    void translatesTypedGoogleHttpAndTransportFailuresWithoutMessageParsing()
    {
        var translator = integration.create("gemini", gemini()).failureTranslator();

        var unavailable = translator.translate(new RuntimeException(
                new ServerException(503, "UNAVAILABLE", "provider overloaded")));
        assertThat(unavailable.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
        assertThat(unavailable.category()).isEqualTo(ProviderFailureCategory.SERVER_ERROR);
        assertThat(unavailable.httpStatus()).isEqualTo(503);
        assertThat(unavailable.providerErrorCode()).isEqualTo("UNAVAILABLE");
        assertThat(unavailable.diagnostics()).singleElement().satisfies(diagnostic ->
                assertThat(diagnostic.get("text")).asString().contains("provider overloaded"));

        var rateLimited = translator.translate(new ClientException(429, "RESOURCE_EXHAUSTED", "slow down"));
        assertThat(rateLimited.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
        assertThat(rateLimited.category()).isEqualTo(ProviderFailureCategory.RATE_LIMITED);

        var connectivity = translator.translate(new GenAiIOException("network failed", new SocketException("reset")));
        assertThat(connectivity.classification()).isEqualTo(ProviderFailureClassification.TRANSIENT);
        assertThat(connectivity.category()).isEqualTo(ProviderFailureCategory.CONNECTIVITY);

        var tls = translator.translate(new GenAiIOException("TLS failed", new SSLException("untrusted")));
        assertThat(tls.classification()).isEqualTo(ProviderFailureClassification.PERMANENT);

        var genericIo = translator.translate(new GenAiIOException("decode failed", new IOException("invalid body")));
        assertThat(genericIo.classification()).isEqualTo(ProviderFailureClassification.UNKNOWN);
    }

    @Test
    void openRouterDiagnosticCaptureIsBoundedAtOneMebibyte() throws Exception
    {
        int limit = SpringAiProviderIntegration.DIAGNOSTIC_LIMIT_BYTES;
        for (int bodySize : new int[] { limit, limit + 1 })
        {
            try (MockWebServer server = new MockWebServer())
            {
                server.enqueue(new MockResponse()
                        .setHeader("Content-Type", "application/json")
                        .setBody(openRouterErrorBody(bodySize)));
                LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
                properties.setDriver(AiDriver.OPENAI);
                properties.setApiKey("gateway-key");
                properties.setBaseUrl(server.url("/v1").toString());
                LoomspanProperties.OpenAiOptions openAi = new LoomspanProperties.OpenAiOptions();
                openAi.setCompatibilityProfile(LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER);
                properties.setOpenai(openAi);
                var runtime = integration.create("openrouter", properties);

                Throwable failure = catchThrowable(() -> runtime.chatModel().call(new Prompt("hello",
                        OpenAiChatOptions.builder().model("routed-model").build())));
                var details = runtime.failureTranslator().translate(failure);

                assertThat(failure).isNotNull();
                assertThat(server.getRequestCount()).isEqualTo(1);
                assertThat(details.diagnostics()).singleElement().satisfies(diagnostic ->
                {
                    assertThat(diagnostic.get("captureLimitBytes")).isEqualTo(limit);
                    assertThat(((String) diagnostic.get("text")).getBytes(java.nio.charset.StandardCharsets.UTF_8))
                            .hasSizeLessThanOrEqualTo(limit);
                    assertThat(diagnostic.get("truncated")).isEqualTo(bodySize > limit);
                });
            }
        }
    }

    @Test
    void rejectedOpenRouterResponsesAreClosed() throws Exception
    {
        String errorCompletion = openRouterErrorBody(512);
        CloseAwareResponse error = new CloseAwareResponse(errorCompletion.getBytes(java.nio.charset.StandardCharsets.UTF_8));
        assertThat(catchThrowable(() -> integration.inspectOpenRouter(error))).isNotNull();
        assertThat(error.closed).isTrue();

        CloseAwareResponse oversized = new CloseAwareResponse(
                "x".repeat(SpringAiProviderIntegration.DIAGNOSTIC_LIMIT_BYTES + 1)
                        .getBytes(java.nio.charset.StandardCharsets.UTF_8));
        assertThat(catchThrowable(() -> integration.inspectOpenRouter(oversized))).isNotNull();
        assertThat(oversized.closed).isTrue();

        CloseAwareResponse malformed = new CloseAwareResponse("not-json".getBytes(java.nio.charset.StandardCharsets.UTF_8));
        assertThat(catchThrowable(() -> integration.inspectOpenRouter(malformed))).isNotNull();
        assertThat(malformed.closed).isTrue();
    }

    @Test
    void oversizedStreamingOpenRouterResponseIsReadOnlyToTheBoundAndClosed()
    {
        int limit = SpringAiProviderIntegration.DIAGNOSTIC_LIMIT_BYTES;
        GuardedOversizedInputStream stream = new GuardedOversizedInputStream(limit * 16L);
        ResponseBody body = new ResponseBody()
        {
            private final BufferedSource source = Okio.buffer(Okio.source(stream));

            @Override public MediaType contentType() { return MediaType.get("application/json"); }
            @Override public long contentLength() { return -1; }
            @Override public BufferedSource source() { return source; }
        };
        Response response = new Response.Builder()
                .request(new Request.Builder().url("https://openrouter.example/v1/chat/completions").build())
                .protocol(Protocol.HTTP_1_1)
                .code(200)
                .message("OK")
                .body(body)
                .build();

        Throwable failure = catchThrowable(() -> integration.inspectOpenRouter(response));

        assertThat(failure).isInstanceOf(ProviderCallException.class);
        assertThat(stream.closed).isTrue();
        assertThat(stream.bytesRead)
                .as("the buffered source may read one segment ahead, but never the complete streaming body")
                .isGreaterThanOrEqualTo(limit + 1L)
                .isLessThanOrEqualTo(limit + 8192L);
        assertThat(stream.bytesRead).isLessThan(stream.virtualLength);
    }

    private static LoomspanProperties.ConnectionProperties gemini()
    {
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.GEMINI);
        properties.setApiKey("test-key");
        return properties;
    }

    private static String openRouterErrorBody(int targetBytes)
    {
        String prefix = "{\"id\":\"chatcmpl-error\",\"object\":\"chat.completion\",\"created\":1,"
                + "\"model\":\"routed-model\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"";
        String suffix = "\"},\"finish_reason\":\"error\",\"error\":{\"message\":\"overloaded\","
                + "\"code\":\"E_OVERLOAD\",\"metadata\":{\"error_type\":\"provider_overloaded\"}}}]}";
        int padding = targetBytes - prefix.length() - suffix.length();
        if (padding < 0)
        {
            throw new IllegalArgumentException("targetBytes is too small");
        }
        return prefix + "x".repeat(padding) + suffix;
    }

    private static final class CloseAwareResponse implements ClientHttpResponse
    {
        private final byte[] body;
        private boolean closed;

        private CloseAwareResponse(byte[] body)
        {
            this.body = body;
        }

        @Override public HttpStatusCode getStatusCode() { return HttpStatus.OK; }
        @Override public String getStatusText() { return "OK"; }
        @Override public void close() { closed = true; }
        @Override public InputStream getBody() { return new ByteArrayInputStream(body); }
        @Override public HttpHeaders getHeaders() { return new HttpHeaders(); }
    }

    private static final class GuardedOversizedInputStream extends InputStream
    {
        private final long virtualLength;
        private long bytesRead;
        private boolean closed;

        private GuardedOversizedInputStream(long virtualLength)
        {
            this.virtualLength = virtualLength;
        }

        @Override
        public int read(byte[] bytes, int offset, int length)
        {
            if (bytesRead >= virtualLength) return -1;
            int count = (int) Math.min(length, virtualLength - bytesRead);
            java.util.Arrays.fill(bytes, offset, offset + count, (byte) 'x');
            bytesRead += count;
            return count;
        }

        @Override
        public int read()
        {
            if (bytesRead >= virtualLength) return -1;
            bytesRead++;
            return 'x';
        }

        @Override
        public void close()
        {
            closed = true;
        }
    }
}
