package com.lokiscale.loomspan.internal.observability.web;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.security.autoconfigure.SecurityAutoConfiguration;
import org.springframework.boot.security.autoconfigure.web.servlet.ServletWebSecurityAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilityPhaseOneIntegrationTest.TestApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                "loomspan.observability.enabled=true",
                "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                "loomspan.skills.locations=classpath:/observability-test/*.yaml",
                "loomspan.connections.local.driver=ollama",
                "loomspan.connections.local.base-url=http://localhost:11434",
                "loomspan.models.test.connection=local",
                "loomspan.models.test.provider-model=test-model"
        })
class ObservabilityPhaseOneIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";
    private final HttpClient client = HttpClient.newHttpClient();
    private final ObjectMapper json = new ObjectMapper();

    @LocalServerPort
    int port;

    @Autowired
    ObservabilityActivationCoordinator activation;

    @Test
    @Timeout(30)
    @DisplayName("WF-FAILED-EXECUTION: status, live handshake, catalog, and exact artifact form one secured lifecycle")
    void availableTerminalArtifactIsAlreadyDownloadableAndLaterUnobtainable(@TempDir Path temporary)
            throws Exception
    {
        HttpResponse<byte[]> instance = get(ObservabilityApiPaths.INSTANCE, "application/json");
        assertThat(instance.statusCode()).isEqualTo(200);
        JsonNode status = json.readTree(instance.body());
        assertThat(status.get("consoleCompatibilityVersion").asText()).isEqualTo("0.1.0-SNAPSHOT");
        String instanceId = status.get("instanceId").asText();

        String activityPath = ObservabilityApiPaths.ACTIVITY
                + "?instanceId=" + URLEncoder.encode(instanceId, StandardCharsets.UTF_8)
                + "&afterCursor=0";
        HttpResponse<java.io.InputStream> activity = client.send(
                request(activityPath, "text/event-stream"),
                HttpResponse.BodyHandlers.ofInputStream());
        assertThat(activity.statusCode()).isEqualTo(200);
        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(activity.body(), StandardCharsets.UTF_8)))
        {
            assertThat(reader.readLine()).isEqualTo("event: handshake");
            assertThat(reader.readLine()).startsWith("data: {");
            assertThat(reader.readLine()).isEmpty();
        }
        activation.runtime().orElseThrow().activityDelivery().close();
        assertThat(activation.runtime().orElseThrow().activityDelivery().admittedCount()).isZero();

        byte[] bytes = "{\"type\":\"TRACE_COMPLETED\"}\n".getBytes(StandardCharsets.UTF_8);
        Path file = Files.write(temporary.resolve("phase-one.ndjson"), bytes);
        activation.runtime().orElseThrow().traces().publish(new FinalizedTraceArtifact(
                "phase-one-trace", "phase-one-session", "test.entry", TraceOutcome.FAILED,
                Instant.now(), file, bytes.length, TracePersistencePolicy.ALWAYS, null));

        HttpResponse<byte[]> catalog = get(ObservabilityApiPaths.TRACES, "application/json");
        assertThat(catalog.statusCode()).isEqualTo(200);
        assertThat(new String(catalog.body(), StandardCharsets.UTF_8))
                .contains("\"traceId\":\"phase-one-trace\"");
        HttpResponse<byte[]> detail = get(
                ObservabilityApiPaths.TRACES + "/phase-one-trace", "application/json");
        assertThat(detail.statusCode()).isEqualTo(200);
        HttpResponse<byte[]> artifact = get(
                ObservabilityApiPaths.TRACES + "/phase-one-trace/artifact",
                "application/x-ndjson");
        assertThat(artifact.statusCode()).isEqualTo(200);
        assertThat(artifact.body()).isEqualTo(bytes);

        Files.delete(file);
        HttpResponse<byte[]> racedDeletion = get(
                ObservabilityApiPaths.TRACES + "/phase-one-trace/artifact",
                "application/x-ndjson");
        assertThat(racedDeletion.statusCode()).isEqualTo(404);
        assertThat(new String(racedDeletion.body(), StandardCharsets.UTF_8))
                .contains("\"code\":\"NOT_FOUND\"");
    }

    private HttpResponse<byte[]> get(String path, String accept) throws Exception
    {
        return client.send(request(path, accept), HttpResponse.BodyHandlers.ofByteArray());
    }

    private HttpRequest request(String path, String accept)
    {
        return HttpRequest.newBuilder(URI.create("http://localhost:" + port + path))
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", accept)
                .GET()
                .build();
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = { SecurityAutoConfiguration.class,
            ServletWebSecurityAutoConfiguration.class })
    static class TestApplication
    {
    }
}
