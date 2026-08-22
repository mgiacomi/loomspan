package com.lokiscale.loomspan.internal.observability.web;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.DisplayName;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.security.autoconfigure.SecurityAutoConfiguration;
import org.springframework.boot.security.autoconfigure.web.servlet.ServletWebSecurityAutoConfiguration;
import org.springframework.boot.webmvc.test.autoconfigure.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;
import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.junit.jupiter.api.io.TempDir;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Instant;
import java.util.List;
import java.util.UUID;

import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.head;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.options;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.header;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;
import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilityRestIntegrationTest.TestApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.MOCK,
        properties = {
                "loomspan.observability.enabled=true",
                "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                "spring.jackson.property-naming-strategy=SNAKE_CASE",
                "loomspan.skills.locations=classpath:/observability-test/*.yaml",
                "loomspan.connections.local.driver=ollama",
                "loomspan.connections.local.base-url=http://localhost:11434",
                "loomspan.models.test.connection=local",
                "loomspan.models.test.provider-model=test-model"
        })
@AutoConfigureMockMvc
class ObservabilityRestIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";
    private final MockMvc mvc;
    private final ObservabilityActivationCoordinator activation;

    @Autowired
    ObservabilityRestIntegrationTest(MockMvc mvc, ObservabilityActivationCoordinator activation)
    {
        this.mvc = mvc;
        this.activation = activation;
    }

    @Test
    void authenticatesAndReturnsExactReleaseIdentityAndNoStore() throws Exception
    {
        mvc.perform(get(ObservabilityApiPaths.INSTANCE))
                .andExpect(status().isUnauthorized())
                .andExpect(header().doesNotExist(ObservabilityApiKeyFilter.INSTANCE_HEADER))
                .andExpect(jsonPath("$.code").value("LOOMSPAN_API_KEY_REJECTED"));

        mvc.perform(get(ObservabilityApiPaths.INSTANCE)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(header().string("Cache-Control", "no-store"))
                .andExpect(header().exists(ObservabilityApiKeyFilter.INSTANCE_HEADER))
                .andExpect(jsonPath("$.consoleCompatibilityVersion").value("0.1.0-SNAPSHOT"))
                .andExpect(jsonPath("$.registeredSkillCount").value(1))
                .andExpect(jsonPath("$.completionGraceTtl").value("PT15M"));
    }

    @Test
    void collectionAndNamespaceFallbackAreStable() throws Exception
    {
        mvc.perform(get(ObservabilityApiPaths.SKILLS)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                        .param("pageSize", "1"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items").isArray())
                .andExpect(jsonPath("$.hasMore").value(false))
                .andExpect(jsonPath("$.nextCursor").doesNotExist());

        mvc.perform(get(ObservabilityApiPaths.SKILLS + "/CheckDns")
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.registeredName").value("CheckDns"))
                .andExpect(jsonPath("$.yaml").value(org.hamcrest.Matchers.containsString("name: CheckDns")));

        mvc.perform(get(ObservabilityApiPaths.ROOT + "/unknown")
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.code").value("NOT_FOUND"));

        mvc.perform(post(ObservabilityApiPaths.INSTANCE)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
    }

    @Test
    void incompatibleOrMalformedAcceptHeaderReturnsInvalidRequest() throws Exception
    {
        for (String accept : List.of(
                MediaType.TEXT_PLAIN_VALUE,
                MediaType.APPLICATION_JSON_VALUE + ";q=0",
                "not/a valid media type"))
        {
            mvc.perform(get(ObservabilityApiPaths.INSTANCE)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .header("Accept", accept))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
        }
    }

    @Test
    void rejectsQueryParametersOnStatusAndDetailResources() throws Exception
    {
        for (String path : List.of(
                ObservabilityApiPaths.INSTANCE,
                ObservabilityApiPaths.SKILLS + "/CheckDns",
                ObservabilityApiPaths.ACTIVE + "/missing",
                ObservabilityApiPaths.TRACES + "/missing"))
        {
            mvc.perform(get(path)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("unexpected", "value"))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
        }
    }

    @Test
    void rejectsHeadOnExactGetResources() throws Exception
    {
        for (var request : List.of(
                head(ObservabilityApiPaths.INSTANCE),
                options(ObservabilityApiPaths.INSTANCE)))
        {
            mvc.perform(request.header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
        }
    }

    @Test
    @DisplayName("WF-SE-R3 WF-SE-R9: active baseline fixes identity, observation time, and replay cursor")
    void activeBaselineCarriesInstanceObservationAndResumeCursor() throws Exception
    {
        mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(header().exists(ObservabilityApiKeyFilter.INSTANCE_HEADER))
                .andExpect(jsonPath("$.observedAt").isString())
                .andExpect(jsonPath("$.resumeCursor").value("0"))
                .andExpect(jsonPath("$.items").isEmpty());
    }

    @Test
    void activityRequestFailuresRemainJsonBeforeAsyncOwnership() throws Exception
    {
        String instanceId = activation.runtime().orElseThrow().instanceId().toString();
        var base = get(ObservabilityApiPaths.ACTIVITY)
                .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                .header("Accept", MediaType.TEXT_EVENT_STREAM_VALUE);

        mvc.perform(base)
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
        mvc.perform(get(ObservabilityApiPaths.ACTIVITY)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                        .header("Accept", MediaType.TEXT_EVENT_STREAM_VALUE)
                        .param("instanceId", instanceId)
                        .param("afterCursor", "-1"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_CURSOR"));
        mvc.perform(get(ObservabilityApiPaths.ACTIVITY)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                        .header("Accept", MediaType.TEXT_EVENT_STREAM_VALUE)
                        .param("instanceId", instanceId)
                        .param("afterCursor", "1"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_CURSOR"));
        mvc.perform(get(ObservabilityApiPaths.ACTIVITY)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                        .header("Accept", MediaType.TEXT_EVENT_STREAM_VALUE)
                        .param("instanceId", UUID.randomUUID().toString())
                        .param("afterCursor", "0"))
                .andExpect(status().isGone())
                .andExpect(jsonPath("$.code").value("STALE_CURSOR"));
        mvc.perform(get(ObservabilityApiPaths.ACTIVITY)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                        .header("Accept", MediaType.TEXT_EVENT_STREAM_VALUE)
                        .header("Last-Event-ID", "0")
                        .param("instanceId", instanceId)
                        .param("afterCursor", "0"))
                .andExpect(status().isBadRequest())
                .andExpect(jsonPath("$.code").value("INVALID_REQUEST"));
    }

    @Test
    void listsAndGetsCurrentActiveExecutionAndFinalizedTrace(@TempDir Path temporaryDirectory) throws Exception
    {
        var runtime = activation.runtime().orElseThrow();
        Instant now = Instant.now(runtime.clock());
        runtime.activeExecutions().replace(new ActiveExecutionSnapshot(
                "session-live", "trace-live", 0, 1, now.minusSeconds(2), now,
                "CheckDns", "RUNNING", "Checking DNS", List.of(), 0, false,
                SessionUsageSnapshot.empty(), null));
        Path artifact = Files.writeString(temporaryDirectory.resolve("trace.ndjson"), "{}\n");
        runtime.traces().publish(new FinalizedTraceArtifact(
                "trace-final", "session-final", "test.entry", TraceOutcome.SUCCEEDED, now, artifact,
                Files.size(artifact), TracePersistencePolicy.ALWAYS, now.plusSeconds(300)));

        mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items[0].sessionId").value("session-live"));
        mvc.perform(get(ObservabilityApiPaths.ACTIVE + "/session-live")
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.status").value("ACTIVE"));
        mvc.perform(get(ObservabilityApiPaths.TRACES)
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.items[0].traceId").value("trace-final"))
                .andExpect(jsonPath("$.items[0].entrySkill").value("test.entry"))
                .andExpect(jsonPath("$.items[0].artifactPath").doesNotExist());
        mvc.perform(get(ObservabilityApiPaths.TRACES + "/trace-final")
                        .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.traceId").value("trace-final"))
                .andExpect(jsonPath("$.entrySkill").value("test.entry"))
                .andExpect(jsonPath("$.catalogOrdinal").doesNotExist());
        runtime.activeExecutions().remove("session-live");
    }

    @Test
    void activeContinuationUsesFreshPageObservationWithoutAtomicMembershipClaim() throws Exception
    {
        var runtime = activation.runtime().orElseThrow();
        try
        {
            Instant now = Instant.now(runtime.clock());
            for (int index = 1; index <= 3; index++)
            {
                runtime.activeExecutions().replace(activeSnapshot("session-" + index, now));
            }

            String firstBody = mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("pageSize", "1"))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.items[0].sessionId").value("session-3"))
                    .andExpect(jsonPath("$.hasMore").value(true))
                    .andReturn().getResponse().getContentAsString();
            var mapper = new tools.jackson.databind.ObjectMapper();
            var firstJson = mapper.readTree(firstBody);
            String nextCursor = firstJson.get("nextCursor").asText();
            Instant firstObservedAt = Instant.parse(firstJson.get("observedAt").asText());
            awaitClockAfter(runtime.clock(), firstObservedAt);

            runtime.activeExecutions().replace(activeSnapshot("session-4", now));
            runtime.activeExecutions().replace(new ActiveExecutionSnapshot(
                    "session-2", "trace-session-2", 0, 2, now.minusSeconds(2), now.plusMillis(1),
                    "CheckDns", "RUNNING", "Checking DNS", List.of(), 0, false,
                    SessionUsageSnapshot.empty(), null));
            runtime.activeExecutions().remove("session-1");
            String secondBody = mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("pageSize", "1")
                            .param("cursor", nextCursor))
                    .andExpect(status().isOk())
                    .andExpect(jsonPath("$.items[0].sessionId").value("session-2"))
                    .andExpect(jsonPath("$.items[0].lastCanonicalSequence").value(2))
                    .andExpect(jsonPath("$.hasMore").value(false))
                    .andReturn().getResponse().getContentAsString();
            var secondJson = mapper.readTree(secondBody);
            assertThat(Instant.parse(secondJson.get("observedAt").asText())).isAfter(firstObservedAt);
            assertThat(secondBody).doesNotContain("session-4", "session-1");

            ObservabilityCursorCodec codec = new ObservabilityCursorCodec(
                    new ObservabilityJsonCodec());
            String stale = codec.encode(
                    ObservabilityCursorCodec.Cursor.initial(UUID.randomUUID(), "active-executions", 3).before(3));
            mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("cursor", stale))
                    .andExpect(status().isGone())
                    .andExpect(jsonPath("$.code").value("STALE_CURSOR"));

            String impossible = codec.encode(
                    ObservabilityCursorCodec.Cursor.initial(
                            runtime.instanceId(), "active-executions", Long.MAX_VALUE).before(Long.MAX_VALUE));
            mvc.perform(get(ObservabilityApiPaths.ACTIVE)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("cursor", impossible))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.code").value("INVALID_CURSOR"));

            String impossibleTrace = codec.encode(
                    ObservabilityCursorCodec.Cursor.initial(
                            runtime.instanceId(), "traces", Long.MAX_VALUE).before(Long.MAX_VALUE));
            mvc.perform(get(ObservabilityApiPaths.TRACES)
                            .header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY)
                            .param("cursor", impossibleTrace))
                    .andExpect(status().isBadRequest())
                    .andExpect(jsonPath("$.code").value("INVALID_CURSOR"));
        }
        finally
        {
            for (int index = 1; index <= 4; index++)
            {
                runtime.activeExecutions().remove("session-" + index);
            }
        }
    }

    private static void awaitClockAfter(Clock clock, Instant observedAt)
    {
        long deadline = System.nanoTime() + 1_000_000_000L;
        while (!Instant.now(clock).isAfter(observedAt))
        {
            if (System.nanoTime() >= deadline)
            {
                throw new AssertionError("Runtime clock did not advance after the first page observation");
            }
            Thread.onSpinWait();
        }
    }

    private static ActiveExecutionSnapshot activeSnapshot(String sessionId, Instant now)
    {
        return new ActiveExecutionSnapshot(
                sessionId, "trace-" + sessionId, 0, 1, now.minusSeconds(2), now,
                "CheckDns", "RUNNING", "Checking DNS", List.of(), 0, false,
                SessionUsageSnapshot.empty(), null);
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = { SecurityAutoConfiguration.class,
            ServletWebSecurityAutoConfiguration.class })
    static class TestApplication
    {
    }
}
