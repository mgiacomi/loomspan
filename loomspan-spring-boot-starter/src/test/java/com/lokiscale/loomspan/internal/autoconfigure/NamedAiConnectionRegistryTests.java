package com.lokiscale.loomspan.internal.autoconfigure;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.provider.AttemptOwnership;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.provider.ProviderFailureDetails;
import com.lokiscale.loomspan.internal.provider.ProviderRetryPolicy;
import com.lokiscale.loomspan.internal.springai.v1_1.SpringAiV11ProviderIntegration;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.beans.factory.DisposableBean;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;
import static org.mockito.Mockito.withSettings;

class NamedAiConnectionRegistryTests {

    @Test
    void constructsAndKeepsDistinctNamedConnectionsUsingTheSameDriver() {
        ChatModel primary = mock(ChatModel.class);
        ChatModel secondary = mock(ChatModel.class);
        SpringAiV11ProviderIntegration integration = mock(SpringAiV11ProviderIntegration.class);

        LoomspanProperties.ConnectionProperties first = connection("http://one.example");
        LoomspanProperties.ConnectionProperties second = connection("http://two.example");
        ProviderConnectionRuntime firstRuntime = runtime(primary);
        ProviderConnectionRuntime secondRuntime = runtime(secondary);
        when(integration.create("primary", first)).thenReturn(firstRuntime);
        when(integration.create("secondary", second)).thenReturn(secondRuntime);
        NamedAiConnectionRegistry registry = new NamedAiConnectionRegistry(
                Map.of("primary", first, "secondary", second), integration);

        assertThat(registry.asMap()).containsEntry("primary", firstRuntime).containsEntry("secondary", secondRuntime);
        assertThat(registry.get("primary")).isNotSameAs(registry.get("secondary"));
    }

    @Test
    void preservesSafeFieldDiagnosticsAndCleansUpModelsBuiltBeforeFailure() throws Exception {
        ChatModel closeable = mock(ChatModel.class, withSettings().extraInterfaces(DisposableBean.class));
        SpringAiV11ProviderIntegration integration = mock(SpringAiV11ProviderIntegration.class);

        LinkedHashMap<String, LoomspanProperties.ConnectionProperties> connections = new LinkedHashMap<>();
        connections.put("first", geminiConnection());
        connections.put("second", geminiConnection());
        when(integration.create("first", connections.get("first"))).thenReturn(runtime(closeable));
        when(integration.create("second", connections.get("second"))).thenThrow(new SafeAiConnectionConfigurationException(
                "loomspan.connections.second.gemini.credentials-uri could not be loaded"));

        assertThatThrownBy(() -> new NamedAiConnectionRegistry(connections, integration))
                .isInstanceOf(SafeAiConnectionConfigurationException.class)
                .hasMessage("loomspan.connections.second.gemini.credentials-uri could not be loaded");
        verify((DisposableBean) closeable).destroy();
    }

    @Test
    void opaqueClientRetryOwnershipRequiresLoomspanRetryToBeDisabled() {
        ChatModel model = mock(ChatModel.class);
        SpringAiV11ProviderIntegration integration = mock(SpringAiV11ProviderIntegration.class);
        LoomspanProperties.ConnectionProperties properties = connection("http://opaque.example");
        ProviderConnectionRuntime opaque = new ProviderConnectionRuntime(model, AiDriver.OLLAMA,
                AttemptOwnership.OPAQUE_CLIENT_RETRIES,
                ProviderRetryPolicy.from(properties.getProviderRetry()), ignored -> ProviderFailureDetails.unknown());
        when(integration.create("opaque", properties)).thenReturn(opaque);

        assertThatThrownBy(() -> new NamedAiConnectionRegistry(Map.of("opaque", properties), integration))
                .isInstanceOf(SafeAiConnectionConfigurationException.class)
                .hasMessageContaining("loomspan.connections.opaque.provider-retry.enabled")
                .hasMessageNotContaining("opaque.example");

        properties.getProviderRetry().setEnabled(false);
        ProviderConnectionRuntime disabled = new ProviderConnectionRuntime(model, AiDriver.OLLAMA,
                AttemptOwnership.OPAQUE_CLIENT_RETRIES,
                ProviderRetryPolicy.from(properties.getProviderRetry()), ignored -> ProviderFailureDetails.unknown());
        when(integration.create("opaque", properties)).thenReturn(disabled);
        assertThat(new NamedAiConnectionRegistry(Map.of("opaque", properties), integration).get("opaque"))
                .isSameAs(disabled);
    }

    private static ProviderConnectionRuntime runtime(ChatModel model) {
        return new ProviderConnectionRuntime(model, AiDriver.OLLAMA, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                ProviderRetryPolicy.from(new LoomspanProperties.ProviderRetryProperties()), ignored -> ProviderFailureDetails.unknown());
    }

    private static LoomspanProperties.ConnectionProperties connection(String baseUrl) {
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.OLLAMA);
        properties.setBaseUrl(baseUrl);
        return properties;
    }

    private static LoomspanProperties.ConnectionProperties geminiConnection() {
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.GEMINI);
        properties.setApiKey("test-key");
        return properties;
    }
}
