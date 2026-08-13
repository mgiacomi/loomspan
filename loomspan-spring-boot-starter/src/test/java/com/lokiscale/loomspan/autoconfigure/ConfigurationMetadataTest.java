package com.lokiscale.loomspan.autoconfigure;

import org.junit.jupiter.api.Test;
import tools.jackson.databind.json.JsonMapper;

import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.StreamSupport;

import static org.assertj.core.api.Assertions.assertThat;

class ConfigurationMetadataTest {

    @Test
    void generatedMetadataDocumentsNamedConnectionSurfaceAndDriverHints() throws Exception {
        try (InputStream stream = Thread.currentThread().getContextClassLoader()
                .getResourceAsStream("META-INF/spring-configuration-metadata.json")) {
            assertThat(stream).isNotNull();
            String metadata = new String(stream.readAllBytes(), StandardCharsets.UTF_8);
            var root = JsonMapper.builder().build().readTree(metadata);
            Set<String> connectionAndModelProperties = StreamSupport.stream(
                            root.path("properties").spliterator(), false)
                    .map(property -> property.path("name").asText())
                    .filter(name -> name.startsWith("loomspan.connections") || name.startsWith("loomspan.models"))
                    .collect(Collectors.toSet());
            assertThat(connectionAndModelProperties).containsExactlyInAnyOrder(
                    "loomspan.connections",
                    "loomspan.connections.*.api-key",
                    "loomspan.connections.*.base-url",
                    "loomspan.connections.*.driver",
                    "loomspan.connections.*.headers",
                    "loomspan.connections.*.openai.compatibility-profile",
                    "loomspan.connections.*.openai.organization-id",
                    "loomspan.connections.*.openai.project-id",
                    "loomspan.connections.*.gemini.vertex-ai",
                    "loomspan.connections.*.gemini.project-id",
                    "loomspan.connections.*.gemini.location",
                    "loomspan.connections.*.gemini.credentials-uri",
                    "loomspan.connections.*.provider-retry.enabled",
                    "loomspan.connections.*.provider-retry.max-attempts",
                    "loomspan.connections.*.provider-retry.initial-backoff",
                    "loomspan.connections.*.provider-retry.multiplier",
                    "loomspan.connections.*.provider-retry.max-backoff",
                    "loomspan.connections.*.provider-retry.jitter",
                    "loomspan.models",
                    "loomspan.models.*.connection",
                    "loomspan.models.*.provider-model",
                    "loomspan.models.*.thinking-levels");
            Map<String, String> descriptions = StreamSupport.stream(
                            root.path("properties").spliterator(), false)
                    .collect(Collectors.toMap(
                            property -> property.path("name").asText(),
                            property -> property.path("description").asText()));
            assertThat(descriptions.get("loomspan.connections.*.base-url")).isEqualTo(
                    "Optional driver service-root override; required for Ollama. OpenAI appends "
                            + "/chat/completions to the configured root, so include /v1 in the root when required "
                            + "by the service. Anthropic appends /v1/messages. Values are not emitted in "
                            + "Loomspan diagnostics.");
            assertThat(descriptions.get("loomspan.connections.*.headers")).isEqualTo(
                    "Sensitive static headers supported only for the OpenAI and Anthropic drivers; use them "
                            + "for supported provider-specific headers such as anthropic-beta.");
            assertThat(metadata)
                    .contains("loomspan.connections.*.driver")
                    .contains("loomspan.connections.*.gemini.credentials-uri")
                    .contains("loomspan.models.*.connection")
                    .contains("loomspan.models.*.thinking-levels")
                    .contains("loomspan.observability.enabled")
                    .contains("loomspan.observability.auth.api-key")
                    .contains("loomspan.observability.completion-grace-ttl")
                    .contains("loomspan.observability.trace-catalog-metadata-ttl")
                    .contains("\"value\": \"openai\"")
                    .doesNotContain("unversioned URLs use /v1/chat/completions")
                    .doesNotContain("headers supported only for the OpenAI driver")
                    .doesNotContain("loomspan.connections.*.openai.chat-" + "completions-path")
                    .doesNotContain("loomspan.connections.*.anthropic." + "completions-path")
                    .doesNotContain("loomspan.connections.*.anthropic." + "version")
                    .doesNotContain("loomspan.connections.*.anthropic." + "beta-version")
                    .doesNotContain("loomspan.models.*.provider\"");
        }
    }
}
