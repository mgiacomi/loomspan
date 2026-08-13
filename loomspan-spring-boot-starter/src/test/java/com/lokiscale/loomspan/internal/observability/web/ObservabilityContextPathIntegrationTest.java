package com.lokiscale.loomspan.internal.observability.web;

import org.junit.jupiter.api.Test;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.security.autoconfigure.SecurityAutoConfiguration;
import org.springframework.boot.security.autoconfigure.web.servlet.ServletWebSecurityAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilityContextPathIntegrationTest.TestApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
        properties = {
                "server.servlet.context-path=/orders",
                "loomspan.observability.enabled=true",
                "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                "loomspan.skills.locations=classpath:/observability-test/*.yaml",
                "loomspan.connections.local.driver=ollama",
                "loomspan.connections.local.base-url=http://localhost:11434",
                "loomspan.models.test.connection=local",
                "loomspan.models.test.provider-model=test-model"
        })
class ObservabilityContextPathIntegrationTest
{
    private static final String KEY = "0123456789abcdef0123456789abcdef";

    @LocalServerPort
    int port;

    @Test
    void servesOnlyBeneathServletContext() throws Exception
    {
        HttpClient client = HttpClient.newHttpClient();
        HttpResponse<String> scoped = client.send(request(
                        "/orders" + ObservabilityApiPaths.INSTANCE, true),
                HttpResponse.BodyHandlers.ofString());
        HttpResponse<String> root = client.send(request(
                        ObservabilityApiPaths.INSTANCE, true),
                HttpResponse.BodyHandlers.ofString());

        assertThat(scoped.statusCode()).isEqualTo(200);
        assertThat(scoped.body()).contains("\"consoleCompatibilityVersion\":\"0.1.0-SNAPSHOT\"");
        assertThat(scoped.headers().firstValue(ObservabilityApiKeyFilter.INSTANCE_HEADER)).isPresent();
        assertThat(root.statusCode()).isEqualTo(404);
        assertThat(root.headers().firstValue(ObservabilityApiKeyFilter.INSTANCE_HEADER)).isEmpty();
    }

    private HttpRequest request(String path, boolean withKey)
    {
        HttpRequest.Builder builder = HttpRequest.newBuilder(URI.create("http://localhost:" + port + path)).GET();
        if (withKey)
        {
            builder.header(ObservabilityApiKeyFilter.API_KEY_HEADER, KEY);
        }
        return builder.build();
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = { SecurityAutoConfiguration.class,
            ServletWebSecurityAutoConfiguration.class })
    static class TestApplication
    {
    }
}
