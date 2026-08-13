package com.lokiscale.loomspan.internal.autoconfigure;

import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.runtime.usage.MicrometerUsageMetricsRecorder;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord;
import com.lokiscale.loomspan.internal.runtime.usage.UsagePrecision;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.boot.test.system.CapturedOutput;
import org.springframework.boot.test.system.OutputCaptureExtension;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

@ExtendWith(OutputCaptureExtension.class)
class SensitiveConnectionDataRedactionTest {

    private static final String API_KEY = "SENTINEL-API-KEY";
    private static final String HEADER_VALUE = "SENTINEL-HEADER-VALUE";
    private static final String BASE_URL = "http://[SENTINEL-BASE-URL";
    private static final String CREDENTIAL_URI = "file:SENTINEL-CREDENTIAL-PATH.json";

    @Test
    void secretsAndTransportDetailsStayOutOfOwnedOutput(CapturedOutput output) throws Exception {
        LoomspanProperties.ConnectionProperties connection = new LoomspanProperties.ConnectionProperties();
        connection.setDriver(AiDriver.OPENAI);
        connection.setApiKey(API_KEY);
        connection.setBaseUrl(BASE_URL);
        connection.setHeaders(Map.of("X-Private", HEADER_VALUE));
        LoomspanProperties.GeminiOptions gemini = new LoomspanProperties.GeminiOptions();
        gemini.setCredentialsUri(CREDENTIAL_URI);

        String registryFailure = registryFailure(connection);

        EffectiveSkillExecutionConfiguration configuration = new EffectiveSkillExecutionConfiguration(
                "framework-model", "safe-connection", AiDriver.OPENAI, "provider-model", null);
        ModelExecutionIdentity identity = ModelExecutionIdentity.from(configuration);
        String serializedMetadata = new ObjectMapper().writeValueAsString(identity.metadata());
        String descriptor = SkillExecutionDescriptor.from(configuration).toString();

        SimpleMeterRegistry meters = new SimpleMeterRegistry();
        new MicrometerUsageMetricsRecorder(meters).recordModelUsage(
                "safe-skill", identity, new ModelUsageRecord(1, 1, 2, UsagePrecision.EXACT, null));
        String meterTags = meters.getMeters().stream().flatMap(meter -> meter.getId().getTags().stream())
                .map(tag -> tag.getKey() + "=" + tag.getValue()).toList().toString();

        new ApplicationContextRunner()
                .withConfiguration(AutoConfigurations.of(
                        ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
                .withPropertyValues(
                        "loomspan.skills.locations=classpath:/skills/none/**/*.yaml",
                        "loomspan.connections.sensitive.driver=openai",
                        "loomspan.connections.sensitive.api-key=" + API_KEY,
                        "loomspan.connections.sensitive.base-url=" + BASE_URL,
                        "loomspan.connections.sensitive.headers.X-Private=" + HEADER_VALUE)
                .run(context -> assertThat(context).hasFailed());

        String allOwnedOutput = String.join("\n",
                connection.toString(), gemini.toString(), registryFailure, serializedMetadata,
                descriptor, meterTags, output.getAll());
        assertThat(allOwnedOutput)
                .doesNotContain(API_KEY, HEADER_VALUE, BASE_URL, CREDENTIAL_URI)
                .contains("safe-connection", "OPENAI");
    }

    private static String registryFailure(LoomspanProperties.ConnectionProperties connection) {
        SpringAiProviderIntegration integration = mock(SpringAiProviderIntegration.class);
        when(integration.create("sensitive", connection))
                .thenThrow(new IllegalStateException(API_KEY + HEADER_VALUE + BASE_URL + CREDENTIAL_URI));
        try {
            new NamedAiConnectionRegistry(Map.of("sensitive", connection), integration);
            throw new AssertionError("Expected connection construction to fail");
        }
        catch (IllegalStateException ex) {
            return exceptionText(ex);
        }
    }

    private static String exceptionText(Exception exception) {
        StringWriter writer = new StringWriter();
        exception.printStackTrace(new PrintWriter(writer));
        return writer.toString();
    }
}
