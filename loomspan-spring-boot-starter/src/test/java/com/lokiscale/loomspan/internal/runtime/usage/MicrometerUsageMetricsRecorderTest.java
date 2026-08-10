package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderRetryDecision;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class MicrometerUsageMetricsRecorderTest {

    @Test
    void emitsMicrometerMetersForModelToolLinterAndGuardrailEvents() {
        SimpleMeterRegistry meterRegistry = new SimpleMeterRegistry();
        MicrometerUsageMetricsRecorder recorder = new MicrometerUsageMetricsRecorder(meterRegistry);

        recorder.recordSkillInvocation("root.skill");
        recorder.recordModelUsage("root.skill",
                new ModelExecutionIdentity("test-model", "test-connection", AiDriver.OPENAI, "provider-model"),
                new ModelUsageRecord(3, 5, 8, UsagePrecision.HEURISTIC, null));
        recorder.recordProviderAttempt("root.skill",
                new ModelExecutionIdentity("test-model", "test-connection", AiDriver.OPENAI, "provider-model"),
                "failed", ProviderFailureCategory.RATE_LIMITED, ProviderRetryDecision.RETRY);
        recorder.recordToolInvocation("root.skill", "tool.one", "success");
        recorder.recordToolAccuracy("root.skill", "regex", "inaccurate");
        recorder.recordLinterOutcome(new LinterOutcome("root.skill", "regex", 2, 1, 2, LinterOutcomeStatus.PASSED, "ok"));
        recorder.recordGuardrailTrip("root.skill", GuardrailType.MAX_MODEL_CALLS);

        assertThat(meterRegistry.get("loomspan.skill.calls").tag("skill", "root.skill").counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.model.calls").tag("skill", "root.skill")
                .tag("connection", "test-connection").tag("driver", "openai")
                .tag("precision", "HEURISTIC").counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.model.usage.units").tag("skill", "root.skill").tag("precision", "HEURISTIC").counter().count()).isEqualTo(8.0d);
        assertThat(meterRegistry.get("loomspan.provider.attempts").tag("skill", "root.skill")
                .tag("model", "test-model").tag("connection", "test-connection").tag("driver", "openai")
                .tag("outcome", "failed").tag("category", "rate_limited").tag("decision", "retry")
                .counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.tool.calls").tag("skill", "root.skill").tag("tool", "tool.one").tag("outcome", "success").counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.tool.accuracy.samples").tag("skill", "root.skill").tag("linter", "regex").tag("outcome", "inaccurate").counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.linter.outcomes").tag("skill", "root.skill").tag("status", "PASSED").tag("linter", "regex").counter().count()).isEqualTo(1.0d);
        assertThat(meterRegistry.get("loomspan.guardrail.trips").tag("skill", "root.skill").tag("guardrail", "MAX_MODEL_CALLS").tag("outcome", "quota_exceeded").counter().count()).isEqualTo(1.0d);
    }
}
