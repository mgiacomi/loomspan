package com.lokiscale.loomspan.sample;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = SampleApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                "server.port=0",
                "loomspan.observability.enabled=true",
                "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef"
        })
class ObservabilitySampleIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";
    private static final String ROOT = "/_loomspan/observability/v1";
    private final HttpClient client = HttpClient.newHttpClient();
    private final ObjectMapper json = new ObjectMapper();

    @LocalServerPort
    int port;

    @Test
    @Timeout(30)
    void mappedSkillProducesAuthenticatedCatalogedDownloadThroughHostPermitAll() throws Exception
    {
        HttpResponse<byte[]> business = send("/expenses", null, null);
        assertThat(business.statusCode()).isEqualTo(200);

        HttpResponse<byte[]> rejected = send(ROOT + "/instance", null, "application/json");
        assertThat(rejected.statusCode()).isEqualTo(401);
        assertThat(new String(rejected.body(), java.nio.charset.StandardCharsets.UTF_8))
                .contains("\"code\":\"LOOMSPAN_API_KEY_REJECTED\"");

        HttpResponse<byte[]> traces = send(ROOT + "/traces", KEY, "application/json");
        assertThat(traces.statusCode()).isEqualTo(200);
        JsonNode items = json.readTree(traces.body()).get("items");
        assertThat(items).isNotNull();
        assertThat(items.isEmpty()).isFalse();
        String traceId = items.get(0).get("traceId").asText();

        HttpResponse<byte[]> artifact = send(
                ROOT + "/traces/" + traceId + "/artifact", KEY, "application/x-ndjson");
        assertThat(artifact.statusCode()).isEqualTo(200);
        assertThat(artifact.body()).isNotEmpty();
        assertThat(artifact.headers().firstValue("Content-Disposition").orElseThrow())
                .contains("loomspan-trace-" + traceId + ".ndjson");
        assertThat(artifact.headers().firstValue("Cache-Control")).hasValue("no-store");
        assertThat(artifact.headers().firstValue("X-loomspan-Instance-Id")).isPresent();
    }

    private HttpResponse<byte[]> send(String path, String key, String accept) throws Exception
    {
        HttpRequest.Builder request = HttpRequest.newBuilder(
                URI.create("http://localhost:" + port + path)).GET();
        if (key != null)
        {
            request.header("X-loomspan-Api-Key", key);
        }
        if (accept != null)
        {
            request.header("Accept", accept);
        }
        return client.send(request.build(), HttpResponse.BodyHandlers.ofByteArray());
    }
}
