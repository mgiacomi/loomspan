package com.lokiscale.loomspan.integration;

import com.lokiscale.loomspan.api.SkillExecutionView;
import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import com.lokiscale.loomspan.api.SkillTemplate;
import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.Map;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;

class SupportedSurfaceIntegrationTest
{
    @Test
    void invokesLlmBackedYamlSkillThroughSupportedSurfaceAndStandardConnectionConfiguration() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse()
                    .setHeader("Content-Type", "application/json")
                    .setBody("""
                            {"id":"chatcmpl-supported-surface","object":"chat.completion","created":1,
                             "model":"integration-model",
                             "choices":[{"index":0,"message":{"role":"assistant","content":null,
                                         "tool_calls":[{"id":"mapped-call","type":"function","function":
                                         {"name":"supportedMappedLeaf","arguments":"{\\\"message\\\":\\\"hello through the public API\\\"}"}}]},
                                         "finish_reason":"tool_calls"}],
                             "usage":{"prompt_tokens":3,"completion_tokens":3,"total_tokens":6}}
                            """));
            server.enqueue(new MockResponse()
                    .setHeader("Content-Type", "application/json")
                    .setBody("""
                            {"id":"chatcmpl-supported-surface-final","object":"chat.completion","created":1,
                             "model":"integration-model",
                             "choices":[{"index":0,"message":{"role":"assistant","content":"supported surface response"},
                                         "finish_reason":"stop"}],
                             "usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}
                            """));

            new ApplicationContextRunner()
                    .withConfiguration(AutoConfigurations.of(
                            ConfigurationPropertiesAutoConfiguration.class,
                            com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                            LoomspanAutoConfiguration.class,
                            com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
                    .withUserConfiguration(SupportedSkillConfiguration.class)
                    .withPropertyValues(
                            "loomspan.skills.locations=classpath:/skills/integration/*.yml",
                            "loomspan.connections.integration.driver=openai",
                            "loomspan.connections.integration.base-url=" + server.url("/v1"),
                            "loomspan.connections.integration.api-key=integration-key",
                            "loomspan.models.integration.connection=integration",
                            "loomspan.models.integration.provider-model=integration-model",
                            "loomspan.session.mission-timeout=20s")
                    .run(context -> {
                        assertThat(context).hasNotFailed();
                        assertThat(context).hasSingleBean(SkillTemplate.class);

                        SkillTemplate skills = context.getBean(SkillTemplate.class);
                        AtomicReference<SkillExecutionView> observed = new AtomicReference<>();

                        assertThat(skills.invoke(
                                "supportedSurfaceSkill",
                                Map.of("message", "hello through the public API"),
                                observed::set))
                                .isEqualTo("supported surface response");

                        assertThat(observed.get()).isNotNull();
                        assertThat(observed.get().sessionId()).isNotBlank();
                        assertThat(observed.get().events()).isNotNull();
                    });

            RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(request).isNotNull();
            assertThat(request.getPath()).isEqualTo("/v1/chat/completions");
            assertThat(request.getHeader("Authorization")).isEqualTo("Bearer integration-key");
            assertThat(request.getBody().readUtf8())
                    .contains("\"model\":\"integration-model\"")
                    .contains("hello through the public API")
                    .contains("supportedMappedLeaf");

            RecordedRequest finalRequest = server.takeRequest(2, TimeUnit.SECONDS);
            assertThat(finalRequest).isNotNull();
            assertThat(finalRequest.getBody().readUtf8())
                    .contains("mapped: hello through the public API");
        }
    }

    @Configuration(proxyBeanMethods = false)
    static class SupportedSkillConfiguration
    {
        @Bean
        SupportedTarget supportedTargetBean()
        {
            return new SupportedTarget();
        }
    }

    static class SupportedTarget
    {
        @SkillMethod(description = "Echo a message through an application-owned mapped leaf.")
        String echo(@SkillParam(description = "Message supplied by the parent skill.") String message)
        {
            return "mapped: " + message;
        }
    }
}
