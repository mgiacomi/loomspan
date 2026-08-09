package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.security.servlet.SecurityAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilityArtifactIntegrationTest.TestApplication.class,
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
class ObservabilityArtifactIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";

    @LocalServerPort
    int port;

    @Autowired
    ObservabilityActivationCoordinator activation;

    @Test
    @Timeout(20)
    void downloadsExactFinalizedArtifactWithRequiredHeaders() throws Exception
    {
        Path fixture = fixtureRoot().resolve("traces/single-attempt-success.ndjson");
        publish("trace-1", fixture);

        HttpResponse<byte[]> response = send(
                ObservabilityApiPaths.TRACES + "/trace-1/artifact",
                "application/x-ndjson");

        assertThat(response.statusCode()).isEqualTo(200);
        assertThat(response.body()).isEqualTo(Files.readAllBytes(fixture));
        assertThat(response.headers().firstValue("Content-Type").orElseThrow().toLowerCase())
                .isEqualTo("application/x-ndjson;charset=utf-8");
        assertThat(response.headers().firstValue("Content-Disposition").orElseThrow())
                .contains("attachment")
                .contains("loomspan-trace-trace-1.ndjson")
                .doesNotContain(fixture.toString());
        assertThat(response.headers().firstValueAsLong("Content-Length")).hasValue(Files.size(fixture));
        assertThat(response.headers().firstValue("Cache-Control")).hasValue("no-store");
        assertThat(response.headers().firstValue(ObservabilityApiKeyFilter.INSTANCE_HEADER))
                .hasValue(activation.runtime().orElseThrow().instanceId().toString());
        assertThat(activation.runtime().orElseThrow().artifactDelivery().admittedCount()).isZero();
    }

    @Test
    void rejectsUnsupportedShapesAndUnknownArtifactsBeforeCommit() throws Exception
    {
        HttpResponse<byte[]> unknown = send(
                ObservabilityApiPaths.TRACES + "/missing/artifact",
                "application/x-ndjson");
        assertThat(unknown.statusCode()).isEqualTo(404);
        assertThat(new String(unknown.body(), java.nio.charset.StandardCharsets.UTF_8))
                .contains("\"code\":\"NOT_FOUND\"");

        HttpResponse<byte[]> incompatible = send(
                ObservabilityApiPaths.TRACES + "/missing/artifact",
                "application/json");
        assertThat(incompatible.statusCode()).isEqualTo(400);
        assertThat(new String(incompatible.body(), java.nio.charset.StandardCharsets.UTF_8))
                .contains("\"code\":\"INVALID_REQUEST\"");
    }

    @ParameterizedTest
    @ValueSource(strings = {
            "single-attempt-success.ndjson",
            "runtime-terminal-failure.ndjson",
            "runtime-terminal-abort.ndjson",
            "advisor-retry.ndjson",
            "chunked-payload.ndjson",
            "unattributed-usage.ndjson",
            "malformed-json.ndjson",
            "incomplete-chunks.ndjson"
    })
    void streamsRepresentativeFixtureByteForByte(String name) throws Exception
    {
        Path fixture = fixtureRoot().resolve("traces").resolve(name);
        String traceId = name.substring(0, name.length() - ".ndjson".length());
        publish(traceId, fixture);

        HttpResponse<byte[]> response = send(
                ObservabilityApiPaths.TRACES + "/" + traceId + "/artifact",
                "application/x-ndjson");

        assertThat(response.statusCode()).isEqualTo(200);
        assertThat(response.body()).isEqualTo(Files.readAllBytes(fixture));
    }

    private void publish(String traceId, Path path) throws Exception
    {
        activation.runtime().orElseThrow().traces().publish(new FinalizedTraceArtifact(
                traceId, "session-" + traceId, "test.entry", TraceOutcome.SUCCEEDED, Instant.now(),
                path, Files.size(path), TracePersistencePolicy.ALWAYS, null));
    }

    private HttpResponse<byte[]> send(String path, String accept) throws Exception
    {
        HttpRequest request = HttpRequest.newBuilder(
                        URI.create("http://localhost:" + port + path))
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", accept)
                .GET()
                .build();
        return HttpClient.newHttpClient().send(request, HttpResponse.BodyHandlers.ofByteArray());
    }

    private static Path fixtureRoot()
    {
        Path cwd = Path.of("").toAbsolutePath();
        Path direct = cwd.resolve("loomspan-console-fixtures");
        return Files.isDirectory(direct) ? direct : cwd.getParent().resolve("loomspan-console-fixtures");
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = SecurityAutoConfiguration.class)
    static class TestApplication
    {
    }
}
