package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivity;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivityKind;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.security.servlet.SecurityAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.test.annotation.DirtiesContext;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Map;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilitySseIntegrationTest.TestApplication.class,
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
@DirtiesContext(classMode = DirtiesContext.ClassMode.AFTER_EACH_TEST_METHOD)
class ObservabilitySseIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";

    @LocalServerPort
    int port;

    @Autowired
    ObservabilityActivationCoordinator activation;

    @Test
    @Timeout(20)
    @DisplayName("WF-X-R6 WF-X-R7: authenticated stream handshakes before replay")
    void opensAuthenticatedStreamWithHandshakeAndReplaysActivityAfterCursor() throws Exception
    {
        var runtime = activation.runtime().orElseThrow();
        ExecutionActivity appended = runtime.replayBuffer().append(new ExecutionActivity(
                0, "session-1", "trace-1", 1L, Instant.parse("2026-07-26T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, "ACTIVE",
                "Execution started", Map.of(), 256));
        String instanceId = runtime.instanceId().toString();
        String query = "?instanceId=" + URLEncoder.encode(instanceId, StandardCharsets.UTF_8)
                + "&afterCursor=0";
        HttpRequest request = HttpRequest.newBuilder(URI.create(
                        "http://localhost:" + port + ObservabilityApiPaths.ROOT + "/activity" + query))
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", "text/event-stream")
                .GET()
                .build();

        HttpResponse<java.io.InputStream> response = HttpClient.newHttpClient()
                .send(request, HttpResponse.BodyHandlers.ofInputStream());
        try (BufferedReader reader = new BufferedReader(
                new InputStreamReader(response.body(), StandardCharsets.UTF_8)))
        {
            assertThat(response.statusCode()).isEqualTo(200);
            assertThat(response.headers().firstValue("Content-Type").orElse(""))
                    .startsWith("text/event-stream");
            assertThat(response.headers().firstValue("Cache-Control")).contains("no-store");
            assertThat(response.headers().firstValue(ObservabilityApiKeyFilter.INSTANCE_HEADER))
                    .contains(instanceId);

            String first = readEvent(reader);
            String second = readEvent(reader);
            assertThat(first).startsWith("event: handshake\n")
                    .doesNotContain("\nid:")
                    .contains("\"instanceId\":\"" + instanceId + "\"")
                    .contains("\"afterCursor\":\"0\"");
            assertThat(second).startsWith("id: " + appended.deliveryCursor() + "\nevent: activity\n")
                    .contains("\"instanceId\":\"" + instanceId + "\"")
                    .contains("\"cursor\":\"" + appended.deliveryCursor() + "\"");

            ExecutionActivity later = runtime.replayBuffer().append(new ExecutionActivity(
                    0, "session-2", "trace-2", 1L, Instant.parse("2026-07-26T12:00:01Z"),
                    ExecutionActivityKind.TRACE_STARTED, null, null, null, null, "ACTIVE",
                    "Later execution", Map.of(), 256));
            HttpResponse<java.io.InputStream> reconnected = HttpClient.newHttpClient().send(
                    streamRequest(instanceId, appended.deliveryCursor()),
                    HttpResponse.BodyHandlers.ofInputStream());
            try (BufferedReader reconnectReader = new BufferedReader(
                    new InputStreamReader(reconnected.body(), StandardCharsets.UTF_8)))
            {
                assertThat(reconnected.statusCode()).isEqualTo(200);
                assertThat(readEvent(reconnectReader)).startsWith("event: handshake\n");
                assertThat(readEvent(reconnectReader))
                        .startsWith("id: " + later.deliveryCursor() + "\nevent: activity\n");
            }
            runtime.activityDelivery().liveUnavailable();
        }
    }

    @Test
    @Timeout(30)
    void rejectsSeventeenthStreamAndReclaimsAllSlotsOnLiveFailure() throws Exception
    {
        var runtime = activation.runtime().orElseThrow();
        String instanceId = runtime.instanceId().toString();
        HttpClient client = HttpClient.newHttpClient();
        List<java.io.InputStream> open = new ArrayList<>();
        try
        {
            for (int index = 0; index < 16; index++)
            {
                HttpResponse<java.io.InputStream> response = client.send(
                        streamRequest(instanceId, 0), HttpResponse.BodyHandlers.ofInputStream());
                assertThat(response.statusCode()).isEqualTo(200);
                open.add(response.body());
            }
            HttpResponse<String> rejected = client.send(
                    streamRequest(instanceId, 0), HttpResponse.BodyHandlers.ofString());

            assertThat(rejected.statusCode()).isEqualTo(429);
            assertThat(rejected.body()).contains("\"code\":\"LIMIT_EXCEEDED\"");
            assertThat(runtime.activityDelivery().admittedCount()).isEqualTo(16);
        }
        finally
        {
            runtime.activityDelivery().liveUnavailable();
            for (java.io.InputStream stream : open)
            {
                stream.close();
            }
        }
    }

    @Test
    @Timeout(20)
    void projectionFailureClosesStreamAndLeavesIndependentOperationsUsable() throws Exception
    {
        var runtime = activation.runtime().orElseThrow();
        String instanceId = runtime.instanceId().toString();
        HttpClient client = HttpClient.newHttpClient();
        HttpResponse<java.io.InputStream> stream = client.send(
                streamRequest(instanceId, 0), HttpResponse.BodyHandlers.ofInputStream());
        assertThat(stream.statusCode()).isEqualTo(200);
        BufferedReader streamReader = new BufferedReader(
                new InputStreamReader(stream.body(), StandardCharsets.UTF_8));
        assertThat(readEvent(streamReader)).startsWith("event: handshake\n");

        runtime.observationFactory().create("failed-session", "test.entry").recordAppended(new TraceRecord(
                "failed-trace",
                "failed-session",
                1,
                Instant.parse("2026-07-26T12:00:03Z"),
                TraceRecordType.TRACE_STARTED,
                "f".repeat(13_000),
                null,
                null,
                null,
                "thread",
                Map.of(),
                null));

        int eof = CompletableFuture.supplyAsync(() ->
        {
            try
            {
                return streamReader.read();
            }
            catch (java.io.IOException failure)
            {
                return -1;
            }
        }).get(5, TimeUnit.SECONDS);
        streamReader.close();
        assertThat(eof).isEqualTo(-1);
        assertThat(runtime.liveMonitoring().isAvailable()).isFalse();
        awaitAdmittedCount(runtime, 0);

        assertThat(client.send(jsonRequest(ObservabilityApiPaths.INSTANCE),
                HttpResponse.BodyHandlers.ofString()).statusCode()).isEqualTo(200);
        assertThat(client.send(jsonRequest(ObservabilityApiPaths.SKILLS),
                HttpResponse.BodyHandlers.ofString()).statusCode()).isEqualTo(200);
        assertThat(client.send(jsonRequest(ObservabilityApiPaths.TRACES),
                HttpResponse.BodyHandlers.ofString()).statusCode()).isEqualTo(200);
        assertThat(client.send(jsonRequest(ObservabilityApiPaths.ACTIVE),
                HttpResponse.BodyHandlers.ofString()).statusCode()).isEqualTo(503);
        HttpResponse<String> rejected = client.send(
                streamRequest(instanceId, 0), HttpResponse.BodyHandlers.ofString());
        assertThat(rejected.statusCode()).isEqualTo(503);
        assertThat(rejected.body()).contains("\"code\":\"LIVE_MONITORING_UNAVAILABLE\"");
    }

    private HttpRequest streamRequest(String instanceId, long afterCursor)
    {
        String query = "?instanceId=" + URLEncoder.encode(instanceId, StandardCharsets.UTF_8)
                + "&afterCursor=" + afterCursor;
        return HttpRequest.newBuilder(URI.create(
                        "http://localhost:" + port + ObservabilityApiPaths.ACTIVITY + query))
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", "text/event-stream")
                .GET()
                .build();
    }

    private HttpRequest jsonRequest(String path)
    {
        return HttpRequest.newBuilder(URI.create("http://localhost:" + port + path))
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", "application/json")
                .GET()
                .build();
    }

    private static void awaitAdmittedCount(
            com.lokiscale.loomspan.internal.observability.ObservabilityRuntime runtime,
            int expected) throws Exception
    {
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5);
        while (runtime.activityDelivery().admittedCount() != expected && System.nanoTime() < deadline)
        {
            Thread.onSpinWait();
        }
        assertThat(runtime.activityDelivery().admittedCount()).isEqualTo(expected);
    }

    private static String readEvent(BufferedReader reader) throws Exception
    {
        StringBuilder event = new StringBuilder();
        for (String line; (line = reader.readLine()) != null;)
        {
            if (line.isEmpty())
            {
                return event.toString();
            }
            if (!event.isEmpty())
            {
                event.append('\n');
            }
            event.append(line);
        }
        throw new AssertionError("stream ended before a complete SSE event");
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = SecurityAutoConfiguration.class)
    static class TestApplication
    {
    }
}
