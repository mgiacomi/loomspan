package com.lokiscale.loomspan.internal.springai.v1_1;

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
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;

import java.io.IOException;
import java.net.SocketException;
import java.util.Optional;
import javax.net.ssl.SSLException;

import static org.assertj.core.api.Assertions.catchThrowable;

import static org.assertj.core.api.Assertions.assertThat;

class SpringAiV11ProviderIntegrationTest
{
    private final SpringAiV11ProviderIntegration integration =
            new SpringAiV11ProviderIntegration(new DefaultResourceLoader());

    @Test
    void googleSdkRetriesAreDisabledAtTheHttpClientBoundary()
    {
        assertThat(SpringAiV11ProviderIntegration.oneAttemptGoogleHttpOptions()
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

    private static LoomspanProperties.ConnectionProperties gemini()
    {
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.GEMINI);
        properties.setApiKey("test-key");
        return properties;
    }
}
