package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.autoconfigure.ExecutionTraceProperties;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.AutoConfigurations;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.context.properties.bind.validation.BindValidationException;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.core.NestedExceptionUtils;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;

class LoomspanSessionPropertiesTest {

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withConfiguration(AutoConfigurations.of(
                    ConfigurationPropertiesAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration.class,
                    LoomspanAutoConfiguration.class,
                    com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration.class))
            .withPropertyValues("loomspan.skills.locations=classpath:/skills/none/**/*.yaml");

    @Test
    void bindsDefaultAndOverriddenSessionProperties() {
        contextRunner.run(context -> {
            LoomspanProperties.Session properties = context.getBean(LoomspanProperties.class).getSession();
            assertThat(properties.getMaxDepth()).isEqualTo(32);
            assertThat(properties.getMissionTimeout()).isEqualTo(Duration.ofSeconds(60));
            assertThat(properties.getQuotas().getMaxSkillInvocations()).isEqualTo(64);
            assertThat(properties.getQuotas().getMaxToolInvocations()).isEqualTo(128);
            assertThat(properties.getQuotas().getMaxLinterRetries()).isEqualTo(32);
            assertThat(properties.getQuotas().getMaxModelCalls()).isEqualTo(64);
            assertThat(properties.getQuotas().getMaxUsageUnits()).isEqualTo(200_000);
            assertThat(context.getBean(ExecutionTraceProperties.class).getPersistence()).isEqualTo(TracePersistencePolicy.ONERROR);
        });

        contextRunner
                .withPropertyValues(
                        "loomspan.session.max-depth=3",
                        "loomspan.session.mission-timeout=5s",
                        "loomspan.session.quotas.max-skill-invocations=4",
                        "loomspan.session.quotas.max-tool-invocations=9",
                        "loomspan.session.quotas.max-linter-retries=7",
                        "loomspan.session.quotas.max-model-calls=5",
                        "loomspan.session.quotas.max-usage-units=1234",
                        "execution-trace.persistence=always")
                .run(context -> {
                    LoomspanProperties.Session properties = context.getBean(LoomspanProperties.class).getSession();
                    ExecutionTraceProperties executionTraceProperties = context.getBean(ExecutionTraceProperties.class);
                    LoomspanSessionRunner runner = context.getBean(LoomspanSessionRunner.class);

                    assertThat(properties.getMaxDepth()).isEqualTo(3);
                    assertThat(properties.getMissionTimeout()).isEqualTo(Duration.ofSeconds(5));
                    assertThat(properties.getQuotas().getMaxSkillInvocations()).isEqualTo(4);
                    assertThat(properties.getQuotas().getMaxToolInvocations()).isEqualTo(9);
                    assertThat(properties.getQuotas().getMaxLinterRetries()).isEqualTo(7);
                    assertThat(properties.getQuotas().getMaxModelCalls()).isEqualTo(5);
                    assertThat(properties.getQuotas().getMaxUsageUnits()).isEqualTo(1234);
                    assertThat(executionTraceProperties.getPersistence()).isEqualTo(TracePersistencePolicy.ALWAYS);
                    assertThat(runner.callWithNewSession("test.entry", LoomspanSession::getMaxDepth)).isEqualTo(3);
                });
    }

    @Test
    void rejectsInvalidMaxDepthValues() {
        contextRunner
                .withPropertyValues("loomspan.session.max-depth=0")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasRootCauseInstanceOf(BindValidationException.class);
                });

        contextRunner
                .withPropertyValues("loomspan.session.mission-timeout=0s")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasRootCauseInstanceOf(IllegalArgumentException.class);
                    assertThat(NestedExceptionUtils.getMostSpecificCause(context.getStartupFailure()))
                            .hasMessageContaining("missionTimeout must be greater than zero");
                });

        contextRunner
                .withPropertyValues("loomspan.session.quotas.max-model-calls=0")
                .run(context -> {
                    assertThat(context).hasNotFailed();
                    assertThat(context.getBean(LoomspanProperties.class).getSession().getQuotas().getMaxModelCalls()).isZero();
                });

        contextRunner
                .withPropertyValues("loomspan.session.quotas.max-model-calls=-1")
                .run(context -> {
                    assertThat(context.getStartupFailure())
                            .isNotNull()
                            .hasRootCauseInstanceOf(BindValidationException.class);
                });
    }
}
