package com.lokiscale.loomspan.internal.autoconfigure;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration;
import org.junit.jupiter.api.Test;
import org.springframework.ai.anthropic.AnthropicChatModel;
import org.springframework.ai.google.genai.GoogleGenAiChatModel;
import org.springframework.ai.ollama.OllamaChatModel;
import org.springframework.ai.openai.OpenAiChatModel;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.ResourceLoader;

import java.nio.charset.StandardCharsets;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class AiConnectionChatModelFactoryTests {

    @Test
    void constructsDistinctOpenAiAndOllamaModelsPerConnection() {
        LoomspanProperties.ConnectionProperties openAi = new LoomspanProperties.ConnectionProperties();
        openAi.setDriver(AiDriver.OPENAI);
        openAi.setApiKey("test-key");
        SpringAiProviderIntegration integration = integration();
        assertThat(integration.create("one", openAi).chatModel()).isInstanceOf(OpenAiChatModel.class)
                .isNotSameAs(integration.create("two", openAi).chatModel());

        LoomspanProperties.ConnectionProperties ollama = new LoomspanProperties.ConnectionProperties();
        ollama.setDriver(AiDriver.OLLAMA);
        ollama.setBaseUrl("http://localhost:11434");
        assertThat(integration.create("one", ollama).chatModel()).isInstanceOf(OllamaChatModel.class)
                .isNotSameAs(integration.create("two", ollama).chatModel());
    }

    @Test
    void constructsAnthropicAndBothGeminiCredentialModes() {
        LoomspanProperties.ConnectionProperties anthropic = new LoomspanProperties.ConnectionProperties();
        anthropic.setDriver(AiDriver.ANTHROPIC);
        anthropic.setApiKey("test-key");
        assertThat(integration().create("anthropic", anthropic).chatModel())
                .isInstanceOf(AnthropicChatModel.class);

        ResourceLoader resourceLoader = mock(ResourceLoader.class);
        when(resourceLoader.getResource("test:credentials")).thenReturn(new ByteArrayResource("""
                {"type":"authorized_user","client_id":"client","client_secret":"secret","refresh_token":"token"}
                """.getBytes(StandardCharsets.UTF_8)));
        SpringAiProviderIntegration geminiFactory = new SpringAiProviderIntegration(resourceLoader);
        LoomspanProperties.ConnectionProperties apiKeyGemini = new LoomspanProperties.ConnectionProperties();
        apiKeyGemini.setDriver(AiDriver.GEMINI);
        apiKeyGemini.setApiKey("test-key");
        assertThat(geminiFactory.create("gemini-key", apiKeyGemini).chatModel()).isInstanceOf(GoogleGenAiChatModel.class);

        LoomspanProperties.ConnectionProperties vertexGemini = new LoomspanProperties.ConnectionProperties();
        vertexGemini.setDriver(AiDriver.GEMINI);
        LoomspanProperties.GeminiOptions vertex = new LoomspanProperties.GeminiOptions();
        vertex.setVertexAi(true);
        vertex.setProjectId("test-project");
        vertex.setLocation("us-central1");
        vertex.setCredentialsUri("test:credentials");
        vertexGemini.setGemini(vertex);
        assertThat(geminiFactory.create("gemini-vertex", vertexGemini).chatModel()).isInstanceOf(GoogleGenAiChatModel.class);
    }

    private static SpringAiProviderIntegration integration() {
        return new SpringAiProviderIntegration(mock(ResourceLoader.class));
    }
}
