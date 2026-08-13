package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.SpringBootConfiguration;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.security.autoconfigure.SecurityAutoConfiguration;
import org.springframework.boot.webmvc.autoconfigure.WebMvcAutoConfiguration;
import org.springframework.boot.test.context.SpringBootTest;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(
        classes = ObservabilityWithoutMvcIntegrationTest.TestApplication.class,
        webEnvironment = SpringBootTest.WebEnvironment.MOCK,
        properties = {
                "loomspan.observability.enabled=true",
                "loomspan.observability.auth.api-key=0123456789abcdef0123456789abcdef",
                "loomspan.skills.locations=classpath:/observability-test/*.yaml",
                "loomspan.connections.local.driver=ollama",
                "loomspan.connections.local.base-url=http://localhost:11434",
                "loomspan.models.test.connection=local",
                "loomspan.models.test.provider-model=test-model"
        })
class ObservabilityWithoutMvcIntegrationTest
{
    @Autowired
    ObservabilityActivationCoordinator activation;

    @Test
    void disablesCleanlyWhenServletApplicationHasNoMvcInfrastructure()
    {
        assertThat(activation.state()).isEqualTo(ObservabilityActivationCoordinator.State.DISABLED);
        assertThat(activation.runtime()).isEmpty();
    }

    @SpringBootConfiguration
    @EnableAutoConfiguration(exclude = {
            SecurityAutoConfiguration.class,
            WebMvcAutoConfiguration.class
    })
    static class TestApplication
    {
    }
}
