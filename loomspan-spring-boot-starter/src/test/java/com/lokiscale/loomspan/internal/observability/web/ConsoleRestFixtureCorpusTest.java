package com.lokiscale.loomspan.internal.observability.web;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class ConsoleRestFixtureCorpusTest
{
    private static final ObjectMapper JSON = JsonMapper.builder().findAndAddModules()
            .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)
            .disable(SerializationFeature.WRITE_DURATIONS_AS_TIMESTAMPS)
            .build();
    private static final Instant OBSERVED = Instant.parse("2026-07-25T12:00:00Z");

    @TempDir
    Path temporaryDirectory;

    @Test
    void generatedCorpusMatchesCommittedFixturesByteForByte() throws Exception
    {
        Path generated = temporaryDirectory.resolve("application-rest");
        Files.createDirectories(generated);
        for (Map.Entry<String, Object> fixture : fixtures().entrySet())
        {
            Files.writeString(generated.resolve(fixture.getKey()),
                    JSON.writeValueAsString(fixture.getValue()) + "\n");
        }
        Path committed = fixtureRoot().resolve("application-rest");
        if (Boolean.getBoolean("loomspan.console.fixtures.regenerate"))
        {
            Files.createDirectories(committed);
            for (Path source : Files.list(generated).toList())
            {
                Files.copy(source, committed.resolve(source.getFileName()),
                        java.nio.file.StandardCopyOption.REPLACE_EXISTING);
            }
        }
        List<String> names = Files.list(generated).map(path -> path.getFileName().toString()).sorted().toList();
        assertThat(Files.list(committed).map(path -> path.getFileName().toString()).sorted().toList())
                .containsExactlyElementsOf(names);
        for (String name : names)
        {
            assertThat(Files.readAllBytes(committed.resolve(name)))
                    .as(name)
                    .isEqualTo(Files.readAllBytes(generated.resolve(name)));
        }
    }

    private static Map<String, Object> fixtures()
    {
        Map<String, Object> result = new LinkedHashMap<>();
        var skill = new ObservabilityDtos.SkillSummary(
                "CheckDns", "classpath:/skills/check-dns.yaml", "skills/CheckDns");
        var usage = new ObservabilityDtos.Usage(1, 0, 0, 1, 10, 5, 15, 1, 0, 0);
        var limits = new ObservabilityDtos.QuotaLimits(64, 128, 32, 64, 200000);
        var active = new ObservabilityDtos.ActiveExecution(
                "session-1", "trace-1", 7, Instant.parse("2026-07-25T11:59:55Z"),
                Instant.parse("2026-07-25T11:59:59Z"), 5000, "CheckDns", "ACTIVE",
                "RUNNING", "Checking DNS", List.of(), 0, false, usage, limits);
        var trace = new ObservabilityDtos.Trace(
                "trace-1", "session-1", "CheckDns", TraceOutcome.SUCCEEDED, OBSERVED, 128,
                TracePersistencePolicy.ONERROR, Instant.parse("2026-07-25T12:15:00Z"));

        result.put("instance-status.json", new ObservabilityDtos.InstanceStatus(
                "11111111-1111-4111-8111-111111111111", "0.1.0-SNAPSHOT", OBSERVED, true,
                1, 1, 1, TracePersistencePolicy.ONERROR, Duration.ofMinutes(15), Duration.ofHours(24)));
        result.put("skills-page.json", new ObservabilityDtos.Page<>(List.of(skill), false, null, OBSERVED));
        result.put("skill-detail.json", new ObservabilityDtos.SkillDetail(
                "CheckDns", "classpath:/skills/check-dns.yaml", "# DNS check\r\nname: CheckDns\r\n"));
        result.put("active-executions-page.json",
                new ObservabilityDtos.ActivePage(List.of(active), false, null, OBSERVED, "9"));
        result.put("active-execution-detail.json", active);
        result.put("traces-page.json", new ObservabilityDtos.Page<>(List.of(trace), false, null, OBSERVED));
        result.put("trace-detail.json", trace);
        result.put("empty-page.json", new ObservabilityDtos.Page<>(List.of(), false, null, OBSERVED));
        result.put("continuation-page.json", new ObservabilityDtos.Page<>(
                List.of(skill), true, "eyJ2ZXJzaW9uIjoxfQ", OBSERVED.plusSeconds(1)));
        problems(result);
        return result;
    }

    private static void problems(Map<String, Object> result)
    {
        result.put("problem-loomspan-api-key-rejected.json", new ObservabilityProblem(
                401, ObservabilityProblem.Code.LOOMSPAN_API_KEY_REJECTED, "loomspan API key was rejected"));
        result.put("problem-invalid-request.json", new ObservabilityProblem(
                400, ObservabilityProblem.Code.INVALID_REQUEST, "The request is invalid"));
        result.put("problem-invalid-cursor.json", new ObservabilityProblem(
                400, ObservabilityProblem.Code.INVALID_CURSOR, "The continuation is invalid"));
        result.put("problem-stale-cursor.json", new ObservabilityProblem(
                410, ObservabilityProblem.Code.STALE_CURSOR,
                "The continuation belongs to another application instance"));
        result.put("problem-not-found.json", new ObservabilityProblem(
                404, ObservabilityProblem.Code.NOT_FOUND, "The requested observability resource was not found"));
        result.put("problem-live-monitoring-unavailable.json", new ObservabilityProblem(
                503, ObservabilityProblem.Code.LIVE_MONITORING_UNAVAILABLE,
                "Live execution monitoring is unavailable"));
        result.put("problem-limit-exceeded.json", new ObservabilityProblem(
                429, ObservabilityProblem.Code.LIMIT_EXCEEDED,
                "The observability response exceeds the configured limit"));
        result.put("problem-application-error.json", new ObservabilityProblem(
                500, ObservabilityProblem.Code.APPLICATION_ERROR,
                "The observability request could not be completed"));
    }

    private static Path fixtureRoot()
    {
        Path cwd = Path.of(System.getProperty("user.dir")).toAbsolutePath();
        Path direct = cwd.resolve("loomspan-console-fixtures");
        return Files.isDirectory(direct) ? direct : cwd.getParent().resolve("loomspan-console-fixtures");
    }
}
