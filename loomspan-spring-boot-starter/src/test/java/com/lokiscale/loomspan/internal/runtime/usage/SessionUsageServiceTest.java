package com.lokiscale.loomspan.internal.runtime.usage;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.OperationType;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.runtime.LoomspanQuotaExceededException;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SessionUsageServiceTest {

    private static final ModelExecutionIdentity IDENTITY =
            new ModelExecutionIdentity("test-model", "test-connection", AiDriver.OPENAI, "provider-model");

    @Test
    void throwsWhenModelCallQuotaExceeded() {
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 10, 1, 100), new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 3);

        service.recordMissionStart(session, "root.skill");
        service.recordModelResponse(session, "root.skill", IDENTITY, new ModelUsageRecord(1, 2, 3, UsagePrecision.EXACT, null));

        assertThatThrownBy(() -> service.recordModelResponse(session, "root.skill", IDENTITY, new ModelUsageRecord(1, 2, 3, UsagePrecision.EXACT, null)))
                .isInstanceOf(LoomspanQuotaExceededException.class)
                .extracting("guardrailType", "limit", "observed")
                .containsExactly(GuardrailType.MAX_MODEL_CALLS, 1L, 2L);
    }

    @Test
    void throwsWhenToolInvocationQuotaExceeded() {
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 1, 10, 10, 100), new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-2", "test.entry", 3);

        service.recordToolCall(session, "root.skill", "tool.one");

        assertThatThrownBy(() -> service.recordToolCall(session, "root.skill", "tool.one"))
                .isInstanceOf(LoomspanQuotaExceededException.class)
                .extracting("guardrailType")
                .isEqualTo(GuardrailType.MAX_TOOL_INVOCATIONS);
    }

    @Test
    void throwsWhenLinterRetryQuotaExceeded() {
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 1, 10, 100), new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-3", "test.entry", 3);

        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 0, 1));

        assertThatThrownBy(() -> service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 1, 2)))
                .isInstanceOf(LoomspanQuotaExceededException.class)
                .extracting("guardrailType", "observed")
                .containsExactly(GuardrailType.MAX_LINTER_RETRIES, 2L);
    }

    @Test
    void snapshotsAccumulatedUsage() {
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 10, 10, 100), new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-4", "test.entry", 3);

        service.recordMissionStart(session, "root.skill");
        service.recordToolCall(session, "root.skill", "tool.one");
        service.recordModelResponse(session, "root.skill", IDENTITY, new ModelUsageRecord(3, 4, 7, UsagePrecision.HEURISTIC, null));
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 0, 1));

        assertThat(service.snapshot(session)).isEqualTo(new SessionUsageSnapshot(1, 1, 1, 1, 0, 3, 4, 7, 0, 1, 0));
    }

    @Test
    void reservesProviderAttemptsAtomicallyAndDoesNotIncrementWhenBlocked() {
        LoomspanProperties.Session.Quotas quotas = quotas(10, 10, 10, 10, 100);
        quotas.setMaxProviderAttempts(2);
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas, new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-provider-attempts", "test.entry", 3);

        service.reserveProviderAttempt(session, "root.skill");
        service.reserveProviderAttempt(session, "root.skill");

        assertThatThrownBy(() -> service.reserveProviderAttempt(session, "root.skill"))
                .isInstanceOf(LoomspanQuotaExceededException.class)
                .extracting("guardrailType", "limit", "observed")
                .containsExactly(GuardrailType.MAX_PROVIDER_ATTEMPTS, 2L, 3L);
        assertThat(service.snapshot(session).providerAttempts()).isEqualTo(2);
    }

    @Test
    void doesNotEnforceDisabledQuotas() {
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(0, 0, 0, 0, 0), new NoOpUsageMetricsRecorder());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-5", "test.entry", 3);

        service.recordMissionStart(session, "root.skill");
        service.recordMissionStart(session, "root.skill");
        service.recordToolCall(session, "root.skill", "tool.one");
        service.recordToolCall(session, "root.skill", "tool.two");
        service.recordModelResponse(session, "root.skill", IDENTITY, new ModelUsageRecord(4, 5, 9, UsagePrecision.HEURISTIC, null));
        service.recordModelResponse(session, "root.skill", IDENTITY, new ModelUsageRecord(4, 5, 9, UsagePrecision.HEURISTIC, null));
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 0, 1));
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 1, 2));

        assertThat(service.snapshot(session)).isEqualTo(new SessionUsageSnapshot(2, 2, 2, 2, 0, 8, 10, 18, 0, 2, 0));
    }

    @Test
    void recordsToolAccuracyFromTerminalLinterOutcomeForCurrentFrame() {
        RecordingUsageMetricsRecorder recorder = new RecordingUsageMetricsRecorder();
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 10, 10, 100), recorder);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-6", "test.entry", 3);
        session.pushFrame(new ExecutionFrame("frame-1", null, OperationType.SKILL, TraceFrameType.SKILL_EXECUTION, "root.skill", Map.of(), Instant.now()));

        service.recordToolCall(session, "root.skill", "tool.one");
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.RETRYING, 0, 1));
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.EXHAUSTED, 1, 2));

        assertThat(recorder.toolAccuracySamples).containsExactly("root.skill|regex|inaccurate");
    }

    @Test
    void recordsToolAccuracyForEachToolInvocationInCurrentFrame() {
        RecordingUsageMetricsRecorder recorder = new RecordingUsageMetricsRecorder();
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 10, 10, 100), recorder);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-6b", "test.entry", 3);
        session.pushFrame(new ExecutionFrame("frame-1", null, OperationType.SKILL, TraceFrameType.SKILL_EXECUTION, "root.skill", Map.of(), Instant.now()));

        service.recordToolCall(session, "root.skill", "tool.one");
        service.recordToolCall(session, "root.skill", "tool.two");
        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.EXHAUSTED, 1, 2));

        assertThat(recorder.toolAccuracySamples)
                .containsExactly("root.skill|regex|inaccurate", "root.skill|regex|inaccurate");
    }

    @Test
    void doesNotRecordToolAccuracyWithoutToolActivityForCurrentFrame() {
        RecordingUsageMetricsRecorder recorder = new RecordingUsageMetricsRecorder();
        DefaultSessionUsageService service = new DefaultSessionUsageService(quotas(10, 10, 10, 10, 100), recorder);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-7", "test.entry", 3);
        session.pushFrame(new ExecutionFrame("frame-1", null, OperationType.SKILL, TraceFrameType.SKILL_EXECUTION, "root.skill", Map.of(), Instant.now()));

        service.recordLinterOutcome(session, outcome(LinterOutcomeStatus.PASSED, 0, 1));

        assertThat(recorder.toolAccuracySamples).isEmpty();
    }

    private static LoomspanProperties.Session.Quotas quotas(int maxSkills, int maxTools, int maxLinterRetries, int maxModelCalls, int maxUsageUnits) {
        LoomspanProperties.Session.Quotas quotas = new LoomspanProperties.Session.Quotas();
        quotas.setMaxSkillInvocations(maxSkills);
        quotas.setMaxToolInvocations(maxTools);
        quotas.setMaxLinterRetries(maxLinterRetries);
        quotas.setMaxModelCalls(maxModelCalls);
        quotas.setMaxUsageUnits(maxUsageUnits);
        return quotas;
    }

    private static LinterOutcome outcome(LinterOutcomeStatus status, int retryCount, int attempt) {
        return new LinterOutcome("root.skill", "regex", attempt, retryCount, 4, status, "Return YAML");
    }

    private static final class RecordingUsageMetricsRecorder implements UsageMetricsRecorder {

        private final java.util.List<String> toolAccuracySamples = new java.util.ArrayList<>();

        @Override
        public void recordSkillInvocation(String skillName) {
        }

        @Override
        public void recordModelUsage(String skillName, ModelExecutionIdentity identity, ModelUsageRecord usageRecord) {
        }

        @Override
        public void recordProviderAttempt(String skillName, ModelExecutionIdentity identity, String outcome,
                com.lokiscale.loomspan.internal.provider.ProviderFailureCategory category,
                com.lokiscale.loomspan.internal.provider.ProviderRetryDecision decision) {
        }

        @Override
        public void recordToolInvocation(String skillName, String toolName, String outcome) {
        }

        @Override
        public void recordToolAccuracy(String skillName, String linterType, String outcome) {
            toolAccuracySamples.add(skillName + "|" + linterType + "|" + outcome);
        }

        @Override
        public void recordLinterOutcome(LinterOutcome outcome) {
        }

        @Override
        public void recordGuardrailTrip(String skillName, GuardrailType guardrailType) {
        }
    }
}
