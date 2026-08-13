package com.lokiscale.loomspan.autoconfigure;

import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.core.env.StandardEnvironment;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LoomspanPropertiesTest {

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
            .withPropertyValues("loomspan.skills.locations=classpath:/skills/none/**/*.yaml")
            .withInitializer(context -> {
                if (context.getEnvironment() instanceof StandardEnvironment env) {
                    env.getPropertySources().remove(StandardEnvironment.SYSTEM_ENVIRONMENT_PROPERTY_SOURCE_NAME);
                }
            });

    @Test
    void bindsDisabledObservabilityDefaultsAndValidExternalizedValues()
    {
        contextRunner.run(context ->
        {
            LoomspanProperties.Observability value =
                    context.getBean(LoomspanProperties.class).getObservability();
            assertThat(value.isEnabled()).isFalse();
            assertThat(value.getCompletionGraceTtl()).isEqualTo(java.time.Duration.ofMinutes(15));
            assertThat(value.getTraceCatalogMetadataTtl()).isEqualTo(java.time.Duration.ofHours(24));
        });

        contextRunner.withPropertyValues(
                "loomspan.observability.enabled=false",
                "loomspan.observability.auth.api-key=secret-sentinel-value-that-is-long",
                "loomspan.observability.completion-grace-ttl=0s",
                "loomspan.observability.trace-catalog-metadata-ttl=2h")
                .run(context ->
                {
                    LoomspanProperties.Observability value =
                            context.getBean(LoomspanProperties.class).getObservability();
                    assertThat(value.getCompletionGraceTtl()).isZero();
                    assertThat(value.getTraceCatalogMetadataTtl()).isEqualTo(java.time.Duration.ofHours(2));
                    assertThat(value.toString()).doesNotContain("secret-sentinel");
                    assertThat(value.getAuth().toString()).doesNotContain("secret-sentinel");
                });
    }

    @Test
    void rejectsSseTuningPropertiesBecauseDeliveryLimitsAreFixed()
    {
        contextRunner
                .withPropertyValues("loomspan.observability.sse.pending=128")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .rootCause()
                        .hasMessageContaining("loomspan.observability.sse.pending"));
    }

    @Test
    void bindsKnownUnifiedRootAndRejectsUnknownConnectionFields() {
        contextRunner
                .withPropertyValues(
                        "loomspan.session.max-depth=7",
                        "loomspan.connections.openai-main.driver=openai",
                        "loomspan.connections.openai-main.api-key=test-key",
                        "loomspan.connections.openrouter.driver=openai",
                        "loomspan.connections.openrouter.api-key=test-key-2",
                        "loomspan.models.fast.connection=openai-main",
                        "loomspan.models.fast.provider-model=gpt-fast",
                        "loomspan.models.routed.connection=openrouter",
                        "loomspan.models.routed.provider-model=gpt-routed")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    LoomspanProperties properties = context.getBean(LoomspanProperties.class);
                    assertThat(properties.getSession().getMaxDepth()).isEqualTo(7);
                    assertThat(properties.getConnections()).containsOnlyKeys("openai-main", "openrouter");
                    assertThat(properties.getModels()).containsOnlyKeys("fast", "routed");
                });

        contextRunner
                .withPropertyValues(
                        "loomspan.connections.primary.driver=openai",
                        "loomspan.connections.primary.api-key=test-key",
                        "loomspan.connections.primary.unknown-transport-field=value")
                .run(context -> assertThat(context.getStartupFailure())
                        .isNotNull()
                        .rootCause()
                        .hasMessageContaining("loomspan.connections.primary.unknown-transport-field"));
    }

    @Test
    void bindsExplicitOpenRouterProfileAndProviderRetryDefaults()
    {
        contextRunner.withPropertyValues(
                "loomspan.connections.openrouter.driver=openai",
                "loomspan.connections.openrouter.api-key=test-key",
                "loomspan.connections.openrouter.openai.compatibility-profile=openrouter")
                .run(context ->
                {
                    assertThat(context).hasNotFailed();
                    LoomspanProperties properties = context.getBean(LoomspanProperties.class);
                    LoomspanProperties.ConnectionProperties connection = properties.getConnections().get("openrouter");
                    assertThat(connection.getOpenai().getCompatibilityProfile())
                            .isEqualTo(LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER);
                    assertThat(connection.getProviderRetry().isEnabled()).isTrue();
                    assertThat(connection.getProviderRetry().getMaxAttempts()).isEqualTo(3);
                    assertThat(connection.getProviderRetry().getInitialBackoff()).isEqualTo(java.time.Duration.ofMillis(500));
                    assertThat(connection.getProviderRetry().getMultiplier()).isEqualTo(2.0d);
                    assertThat(connection.getProviderRetry().getMaxBackoff()).isEqualTo(java.time.Duration.ofSeconds(5));
                    assertThat(connection.getProviderRetry().getJitter()).isEqualTo(0.2d);
                    assertThat(properties.getSession().getQuotas().getMaxProviderAttempts()).isEqualTo(192);
                });
    }

    @Test
    void rejectsRemovedProviderFieldsWithTheirFullPaths()
    {
        for (String removed : java.util.List.of(
                "openai.chat-" + "completions-path=/custom/chat/completions",
                "anthropic." + "completions-path=/custom/messages",
                "anthropic." + "version=2026-01-01",
                "anthropic." + "beta-version=test-beta")) {
            String driver = removed.startsWith("openai") ? "openai" : "anthropic";
            String fullPath = "loomspan.connections.removed." + removed.substring(0, removed.indexOf('='));
            contextRunner.withPropertyValues(
                    "loomspan.connections.removed.driver=" + driver,
                    "loomspan.connections.removed.api-key=test-key",
                    "loomspan.connections.removed." + removed)
                    .run(context -> assertThat(context.getStartupFailure())
                            .hasRootCauseInstanceOf(org.springframework.boot.context.properties.bind.UnboundConfigurationPropertiesException.class)
                            .rootCause().hasMessageContaining(fullPath));
        }
    }

    @Test
    void rejectsRemovedProviderAndUnknownConnectionReferencesWithExactPaths() {
        contextRunner
                .withPropertyValues(
                        "loomspan.connections.primary.driver=openai",
                        "loomspan.connections.primary.api-key=test-key",
                        "loomspan.models.fast.provider=openai",
                        "loomspan.models.fast.connection=primary",
                        "loomspan.models.fast.provider-model=gpt-fast")
                .run(context -> assertThat(context.getStartupFailure()).rootCause()
                        .hasMessageContaining("loomspan.models.fast.provider"));

        contextRunner
                .withPropertyValues(
                        "loomspan.models.fast.connection=missing",
                        "loomspan.models.fast.provider-model=gpt-fast")
                .run(context -> assertThat(context.getStartupFailure()).rootCause()
                        .hasMessageContaining("loomspan.models.fast.connection")
                        .hasMessageNotContaining("test-key"));
    }

    @Test
    void validatesDriverSpecificFieldsAndRedactsSensitiveConnectionValues() {
        LoomspanProperties properties = new LoomspanProperties();
        LoomspanProperties.ConnectionProperties connection = new LoomspanProperties.ConnectionProperties();
        connection.setDriver(AiDriver.OLLAMA);
        connection.setBaseUrl("https://secret-endpoint.example");
        connection.setApiKey("secret-api-key");
        connection.setHeaders(java.util.Map.of("X-Secret", "secret-header-value"));
        properties.setConnections(java.util.Map.of("local", connection));

        assertThatThrownBy(properties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.local.api-key")
                .hasMessageNotContaining("secret-api-key")
                .hasMessageNotContaining("secret-header-value")
                .hasMessageNotContaining("secret-endpoint.example");

        connection.setApiKey(null);
        assertThatThrownBy(properties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.local.headers")
                .hasMessageNotContaining("secret-header-value")
                .hasMessageNotContaining("secret-endpoint.example");
        assertThat(connection.toString())
                .doesNotContain("secret-api-key", "secret-header-value", "secret-endpoint.example");
    }

    @Test
    void rejectsCommonFieldsThatTheSelectedDriverDoesNotConsume() {
        LoomspanProperties.ConnectionProperties gemini = new LoomspanProperties.ConnectionProperties();
        gemini.setDriver(AiDriver.GEMINI);
        gemini.setApiKey("test-key");
        gemini.setBaseUrl("https://internal-gemini.example");

        LoomspanProperties geminiProperties = new LoomspanProperties();
        geminiProperties.setConnections(java.util.Map.of("internal", gemini));
        assertThatThrownBy(geminiProperties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.internal.base-url")
                .hasMessageContaining("not supported for driver GEMINI")
                .hasMessageNotContaining("internal-gemini.example");

        LoomspanProperties.ConnectionProperties ollama = new LoomspanProperties.ConnectionProperties();
        ollama.setDriver(AiDriver.OLLAMA);
        ollama.setBaseUrl("http://localhost:11434");
        ollama.setApiKey("ignored-secret");

        LoomspanProperties ollamaProperties = new LoomspanProperties();
        ollamaProperties.setConnections(java.util.Map.of("local", ollama));
        assertThatThrownBy(ollamaProperties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.local.api-key")
                .hasMessageContaining("not supported for driver OLLAMA")
                .hasMessageNotContaining("ignored-secret");
    }

    @Test
    void acceptsCommonHeadersOnlyForOpenAiAndAnthropic()
    {
        for (AiDriver driver : java.util.List.of(AiDriver.OPENAI, AiDriver.ANTHROPIC))
        {
            LoomspanProperties.ConnectionProperties connection = new LoomspanProperties.ConnectionProperties();
            connection.setDriver(driver);
            connection.setApiKey("test-key");
            connection.setHeaders(java.util.Map.of("X-Provider-Feature", "enabled"));

            LoomspanProperties properties = new LoomspanProperties();
            properties.setConnections(java.util.Map.of(driver.name().toLowerCase(), connection));
            assertThatCode(properties::afterPropertiesSet).doesNotThrowAnyException();
        }

        for (AiDriver driver : java.util.List.of(AiDriver.GEMINI, AiDriver.OLLAMA))
        {
            LoomspanProperties.ConnectionProperties connection = new LoomspanProperties.ConnectionProperties();
            connection.setDriver(driver);
            connection.setHeaders(java.util.Map.of("X-Provider-Feature", "enabled"));
            if (driver == AiDriver.GEMINI)
            {
                connection.setApiKey("test-key");
            }
            else
            {
                connection.setBaseUrl("http://localhost:11434");
            }

            LoomspanProperties properties = new LoomspanProperties();
            properties.setConnections(java.util.Map.of(driver.name().toLowerCase(), connection));
            assertThatThrownBy(properties::afterPropertiesSet)
                    .hasMessageContaining(".headers")
                    .hasMessageContaining("only supported for drivers OPENAI and ANTHROPIC");
        }
    }

    @Test
    void validatesGeminiCredentialModesWithoutIgnoringTypedOptions() {
        LoomspanProperties.ConnectionProperties mixedMode = new LoomspanProperties.ConnectionProperties();
        mixedMode.setDriver(AiDriver.GEMINI);
        mixedMode.setApiKey("test-key");
        LoomspanProperties.GeminiOptions vertex = new LoomspanProperties.GeminiOptions();
        vertex.setVertexAi(true);
        vertex.setProjectId("project");
        vertex.setLocation("us-central1");
        mixedMode.setGemini(vertex);

        LoomspanProperties mixedProperties = new LoomspanProperties();
        mixedProperties.setConnections(java.util.Map.of("mixed", mixedMode));
        assertThatThrownBy(mixedProperties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.mixed")
                .hasMessageContaining("exactly one");

        LoomspanProperties.ConnectionProperties ignoredOptions = new LoomspanProperties.ConnectionProperties();
        ignoredOptions.setDriver(AiDriver.GEMINI);
        ignoredOptions.setApiKey("test-key");
        ignoredOptions.setGemini(new LoomspanProperties.GeminiOptions());

        LoomspanProperties ignoredProperties = new LoomspanProperties();
        ignoredProperties.setConnections(java.util.Map.of("api", ignoredOptions));
        assertThatThrownBy(ignoredProperties::afterPropertiesSet)
                .hasMessageContaining("loomspan.connections.api.gemini")
                .hasMessageContaining("only supported when gemini.vertex-ai=true");
    }
}
