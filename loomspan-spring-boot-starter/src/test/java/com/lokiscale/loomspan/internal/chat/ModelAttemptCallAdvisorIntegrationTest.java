package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.core.AdvisorTraceFact;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.SessionContextRunner;
import com.lokiscale.loomspan.internal.core.TestLoomspanSessions;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.LoomspanQuotaExceededException;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.DefaultSessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.MicrometerUsageMetricsRecorder;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpUsageMetricsRecorder;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.metadata.ChatResponseMetadata;
import org.springframework.ai.chat.metadata.DefaultUsage;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Queue;
import java.util.regex.Pattern;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ModelAttemptCallAdvisorIntegrationTest
{
    private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-07-24T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void recordsEachAdvisorRetryAsOnePhysicalAttemptInTheSameRetrySequence()
    {
        SimpleMeterRegistry meterRegistry = new SimpleMeterRegistry();
        DefaultSessionUsageService usageService = new DefaultSessionUsageService(
                new LoomspanProperties().getSession().getQuotas(),
                new MicrometerUsageMetricsRecorder(meterRegistry));
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("attempt-retry", "test.entry", 4);
        ExecutionFrame root = stateService.openMissionFrame(session, "test.skill", Map.of());
        ExecutionFrame modelFrame = stateService.openFrame(
                session,
                TraceFrameType.MODEL_CALL,
                "test.skill#model",
                Map.of());
        ModelTraceContext traceContext = traceContext();
        QueueChatModel model = new QueueChatModel(List.of(
                response("invalid", 10, 4),
                response("OK: corrected", 8, 3)));

        LinterCallAdvisor linter = new LinterCallAdvisor(
                "test.skill",
                "regex",
                Pattern.compile("^OK:.*$"),
                "Return an OK response.",
                1,
                outcome -> stateService.recordLinterOutcome(LoomspanSession.getCurrentSession(), outcome),
                fact -> recordFact(stateService, fact));
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(
                        linter,
                        new ModelAttemptCallAdvisor(stateService, new ModelUsageExtractor(), usageService))
                .build();

        String content = SessionContextRunner.callWithSession(session, () -> client.prompt()
                .system("system")
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                .call()
                .content());

        stateService.closeFrame(session, modelFrame, Map.of("status", "completed"));
        stateService.closeMissionFrame(session, root);

        assertThat(content).isEqualTo("OK: corrected");
        assertThat(model.calls).isEqualTo(2);
        List<TraceRecord> records = records(session);
        List<TraceRecord> responses = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED)
                .toList();
        assertThat(responses).hasSize(2);
        assertThat(responses).extracting(record -> record.metadata().get("attemptNumber"))
                .containsExactly(1, 2);
        assertThat(responses).extracting(record -> record.metadata().get("retrySequenceId"))
                .containsOnly(traceContext.retrySequenceId());
        assertThat(responses).extracting(record -> record.metadata().get("attemptId"))
                .doesNotHaveDuplicates();
        assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isEqualTo(2);
        assertThat(session.getSessionUsage().orElseThrow().promptUnits()).isEqualTo(18);
        assertThat(session.getSessionUsage().orElseThrow().completionUnits()).isEqualTo(7);
        assertThat(meterRegistry.get("loomspan.model.calls")
                .tag("skill", "test.skill")
                .tag("connection", "test-connection")
                .tag("driver", "openai")
                .tag("precision", "EXACT")
                .counter()
                .count()).isEqualTo(2.0d);

        TraceRecord retryFact = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED)
                .filter(record -> "retrying".equals(record.metadata().get("status")))
                .findFirst()
                .orElseThrow();
        assertThat(retryFact.metadata())
                .containsEntry("retrySequenceId", traceContext.retrySequenceId())
                .containsEntry("attemptNumber", 1)
                .containsEntry("attemptId", responses.getFirst().metadata().get("attemptId"));
    }

    @Test
    void recordsPreparedAndSentButNoResponseWhenProviderThrows()
    {
        DefaultSessionUsageService usageService = usageService();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("attempt-throw", "test.entry", 4);
        ExecutionFrame root = stateService.openMissionFrame(session, "test.skill", Map.of());
        ExecutionFrame modelFrame = stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        ModelTraceContext traceContext = traceContext();
        ChatModel throwingModel = prompt ->
        {
            throw new IllegalStateException("provider failed");
        };
        ChatClient client = ChatClient.builder(throwingModel)
                .defaultAdvisors(new ModelAttemptCallAdvisor(stateService, new ModelUsageExtractor(), usageService))
                .build();

        assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                .call()
                .content()))
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("provider failed");

        List<TraceRecord> records = records(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_PREPARED)).hasSize(1);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT)).hasSize(1);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED);
        assertThat(session.getSessionUsage()
                .orElse(com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty())
                .modelCalls()).isZero();

        stateService.closeFrame(session, modelFrame, Map.of("status", "failed"));
        stateService.closeMissionFrame(session, root);
    }

    @Test
    void retryQuotaIsEnforcedFromTheSamePhysicalAttemptsThatAreTraced()
    {
        LoomspanProperties properties = new LoomspanProperties();
        properties.getSession().getQuotas().setMaxModelCalls(1);
        DefaultSessionUsageService usageService = new DefaultSessionUsageService(
                properties.getSession().getQuotas(),
                new NoOpUsageMetricsRecorder());
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("attempt-quota", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        ModelTraceContext traceContext = traceContext();
        QueueChatModel model = new QueueChatModel(List.of(
                response("invalid", 10, 4),
                response("OK: corrected", 8, 3)));
        LinterCallAdvisor linter = new LinterCallAdvisor(
                "test.skill",
                "regex",
                Pattern.compile("^OK:.*$"),
                "Return an OK response.",
                1,
                outcome -> stateService.recordLinterOutcome(LoomspanSession.getCurrentSession(), outcome),
                fact -> recordFact(stateService, fact));
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(
                        linter,
                        new ModelAttemptCallAdvisor(stateService, new ModelUsageExtractor(), usageService))
                .build();

        assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                .call()
                .content()))
                .isInstanceOf(LoomspanQuotaExceededException.class);

        assertThat(model.calls).isEqualTo(2);
        assertThat(records(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED))
                .hasSize(2);
        assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isEqualTo(2);
    }

    private static void recordFact(DefaultExecutionStateService stateService, AdvisorTraceFact fact)
    {
        if (fact.direction() == AdvisorTraceFact.Direction.REQUEST)
        {
            stateService.recordAdvisorRequestMutation(LoomspanSession.getCurrentSession(), fact.context(), fact.attributes());
        }
        else
        {
            stateService.recordAdvisorResponseMutation(LoomspanSession.getCurrentSession(), fact.context(), fact.attributes());
        }
    }

    private static DefaultSessionUsageService usageService()
    {
        return new DefaultSessionUsageService(
                new LoomspanProperties().getSession().getQuotas(),
                new NoOpUsageMetricsRecorder());
    }

    private static ModelTraceContext traceContext()
    {
        return new ModelTraceContext(
                new ModelExecutionIdentity("test-model", "test-connection", AiDriver.OPENAI, "provider/model"),
                "test.skill",
                "mission");
    }

    private static ChatResponse response(String text, int promptUnits, int completionUnits)
    {
        return new ChatResponse(
                List.of(new Generation(new AssistantMessage(text))),
                ChatResponseMetadata.builder()
                        .usage(new DefaultUsage(promptUnits, completionUnits, promptUnits + completionUnits))
                        .build());
    }

    private static List<TraceRecord> records(LoomspanSession session)
    {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static final class QueueChatModel implements ChatModel
    {
        private final Queue<ChatResponse> responses;
        private int calls;

        private QueueChatModel(List<ChatResponse> responses)
        {
            this.responses = new ArrayDeque<>(responses);
        }

        @Override
        public ChatResponse call(org.springframework.ai.chat.prompt.Prompt prompt)
        {
            calls++;
            return responses.remove();
        }
    }
}
