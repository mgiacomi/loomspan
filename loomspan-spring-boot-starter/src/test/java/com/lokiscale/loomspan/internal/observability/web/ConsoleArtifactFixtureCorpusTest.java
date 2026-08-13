package com.lokiscale.loomspan.internal.observability.web;

import tools.jackson.core.util.DefaultIndenter;
import tools.jackson.core.util.DefaultPrettyPrinter;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.SerializationFeature;
import tools.jackson.databind.json.JsonMapper;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.mock.web.MockHttpServletResponse;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class ConsoleArtifactFixtureCorpusTest
{
    private static final ObjectMapper JSON = buildCanonicalJson();

    private static ObjectMapper buildCanonicalJson()
    {
        DefaultPrettyPrinter printer = new DefaultPrettyPrinter();
        DefaultIndenter indenter = new DefaultIndenter("  ", "\n");
        printer.indentObjectsWith(indenter);
        printer.indentArraysWith(indenter);
        return JsonMapper.builder().findAndAddModules()
                .enable(SerializationFeature.INDENT_OUTPUT)
                .defaultPrettyPrinter(printer)
                .build();
    }

    @TempDir
    Path temporaryDirectory;

    @Test
    void generatedArtifactTransportMetadataReferencesCanonicalTraceCorpus() throws Exception
    {
        Path fixtures = ConsoleSseFixtureCorpusTest.fixtureRoot();
        Path body = fixtures.resolve("traces/single-attempt-success.ndjson");
        JsonNode status = JSON.readTree(fixtures.resolve("application-rest/instance-status.json").toFile());
        MockHttpServletResponse response = new MockHttpServletResponse();
        ObservabilityRestController.prepareArtifactResponse(
                response, "single-attempt-success", Files.size(body));

        Map<String, Object> headers = new LinkedHashMap<>();
        headers.put("Content-Type", response.getContentType());
        headers.put("Content-Disposition", response.getHeader("Content-Disposition"));
        headers.put("Content-Length", Files.size(body));
        headers.put("Cache-Control", "no-store");
        headers.put("X-loomspan-Instance-Id", status.get("instanceId").asText());
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("consoleCompatibilityVersion", status.get("consoleCompatibilityVersion").asText());
        metadata.put("method", "GET");
        metadata.put("path",
                "/_loomspan/observability/v1/traces/single-attempt-success/artifact");
        metadata.put("status", 200);
        metadata.put("headers", headers);
        metadata.put("bodyFixture", "../traces/single-attempt-success.ndjson");

        Path generated = temporaryDirectory.resolve("application-artifact");
        Files.createDirectories(generated);
        Files.writeString(generated.resolve("download-response.json"),
                JSON.writeValueAsString(metadata) + "\n");
        ConsoleSseFixtureCorpusTest.compareOrRegenerate(
                generated, fixtures.resolve("application-artifact"));

        assertThat(Files.readAllBytes(body)).hasSize(Math.toIntExact(Files.size(body)));
        assertThat(Files.list(fixtures.resolve("application-artifact")))
                .noneMatch(path -> path.getFileName().toString().endsWith(".ndjson"));
    }
}
