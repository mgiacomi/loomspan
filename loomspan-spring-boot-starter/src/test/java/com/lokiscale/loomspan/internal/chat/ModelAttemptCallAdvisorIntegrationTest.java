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
import com.lokiscale.loomspan.internal.provider.*;
import com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.metadata.ChatResponseMetadata;
import org.springframework.ai.chat.metadata.DefaultUsage;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.openai.OpenAiChatOptions;
import org.springframework.core.io.DefaultResourceLoader;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.function.FunctionToolCallback;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.io.InterruptedIOException;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Queue;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.CancellationException;
import java.util.concurrent.TimeUnit;
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
                        new ProviderAttemptCallAdvisor(runtime(model), stateService, new ModelUsageExtractor(), usageService))
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
    void semanticRetriesWrapOneObservableAi2ToolLoop() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(openAiToolCall("call-1", "first"));
            server.enqueue(openAiText("invalid"));
            server.enqueue(openAiToolCall("call-2", "second"));
            server.enqueue(openAiText("OK: corrected"));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("test-key");
            properties.setBaseUrl(server.url("/v1").toString());
            properties.getProviderRetry().setEnabled(false);
            ProviderConnectionRuntime runtime = new SpringAiProviderIntegration(new DefaultResourceLoader())
                    .create("openai", properties);
            DefaultSessionUsageService usageService = usageService();
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
            LoomspanSession session = TestLoomspanSessions.withId("advisor-recursion", "test.entry", 4);
            stateService.openMissionFrame(session, "test.skill", Map.of());
            stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
            AtomicInteger toolExecutions = new AtomicInteger();
            List<String> toolValues = new ArrayList<>();
            ToolCallback tool = FunctionToolCallback.<Map<String, Object>, String>builder(
                            "lookup",
                            (arguments, context) ->
                            {
                                toolExecutions.incrementAndGet();
                                toolValues.add(String.valueOf(arguments.get("value")));
                                return "looked-up-" + arguments.get("value");
                            })
                    .description("Look up a value")
                    .inputType(new ParameterizedTypeReference<Map<String, Object>>() { })
                    .inputSchema("{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}},\"required\":[\"value\"]}")
                    .build();
            LinterCallAdvisor semanticPolicy = new LinterCallAdvisor(
                    "test.skill", "regex", Pattern.compile("^OK:.*$"),
                    "Return an OK response.", 1, ignored -> { });
            ChatClient client = ChatClient.builder(runtime.chatModel())
                    .defaultAdvisors(semanticPolicy,
                            new ProviderAttemptCallAdvisor(runtime, stateService,
                                    new ModelUsageExtractor(), usageService))
                    .build();

            String content = SessionContextRunner.callWithSession(session, () -> client.prompt()
                    .user("user")
                    .options(OpenAiChatOptions.builder().model("gpt-test"))
                    .toolCallbacks(tool)
                    .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext()))
                    .call()
                    .content());

            assertThat(content).isEqualTo("OK: corrected");
            assertThat(server.getRequestCount()).isEqualTo(4);
            assertThat(toolExecutions).hasValue(2);
            assertThat(toolValues).containsExactly("first", "second");
            assertThat(server.takeRequest().getBody().readUtf8()).contains("\"name\":\"lookup\"");
            assertThat(server.takeRequest().getBody().readUtf8()).contains("\"tool_call_id\":\"call-1\"");
            assertThat(server.takeRequest().getBody().readUtf8()).contains("\"name\":\"lookup\"");
            assertThat(server.takeRequest().getBody().readUtf8()).contains("\"tool_call_id\":\"call-2\"");

            // Semantic validation wraps the AI 2 tool advisor; every inner model turn crosses the
            // physical-attempt advisor exactly once.
            assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(4);
            assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isEqualTo(4);
            assertThat(records(session).stream()
                    .filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT)).hasSize(4);
            assertThat(records(session).stream()
                    .filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED)).hasSize(4);
        }
    }

    @Test
    void recordsSentButNoResponseWhenProviderThrows()
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
                .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime(throwingModel), stateService, new ModelUsageExtractor(), usageService))
                .build();

        assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                .call()
                .content()))
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("provider failed");

        List<TraceRecord> records = records(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT)).hasSize(1);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED);
        assertThat(session.getSessionUsage()
                .orElse(com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty())
                .modelCalls()).isZero();

        stateService.closeFrame(session, modelFrame, Map.of("status", "failed"));
        stateService.closeMissionFrame(session, root);
    }

    @Test
    void retriesTransientProviderFailuresAsDistinctPhysicalAttempts()
    {
        DefaultSessionUsageService usageService = usageService();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("provider-retry", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        ModelTraceContext traceContext = traceContext();
        AtomicInteger calls = new AtomicInteger();
        ChatModel model = prompt ->
        {
            if (calls.incrementAndGet() == 1) throw new IllegalStateException("temporary provider failure");
            return response("OK", 3, 2);
        };
        ProviderFailureDetails transientFailure = new ProviderFailureDetails(
                ProviderFailureClassification.TRANSIENT, ProviderFailureCategory.SERVER_ERROR,
                503, null, null, null, "Provider temporarily unavailable", List.of());
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(retryingRuntime(model, transientFailure),
                        stateService, new ModelUsageExtractor(), usageService))
                .build();

        String content = SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                .call()
                .content());

        assertThat(content).isEqualTo("OK");
        assertThat(calls).hasValue(2);
        assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(2);
        assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isEqualTo(1);
        assertThat(records(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT)).hasSize(2);
        List<TraceRecord> attemptFailures = records(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_ATTEMPT_FAILED)
                .toList();
        assertThat(attemptFailures).singleElement().satisfies(record -> assertThat(record.metadata())
                .containsEntry("attemptReason", "INITIAL")
                .containsEntry("providerAttemptNumber", 1)
                .containsEntry("failureClassification", "TRANSIENT")
                .containsEntry("retryDecision", "RETRY"));
        assertThat(records(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED)
                .findFirst().orElseThrow().metadata())
                .containsEntry("attemptReason", "PROVIDER_RETRY")
                .containsEntry("providerAttemptNumber", 2);
    }

    @Test
    void recordsTranslatedOpenAiReadTimeoutAsRetryableAttemptFact()
    {
        DefaultSessionUsageService usageService = usageService();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("openai-read-timeout", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        AtomicInteger calls = new AtomicInteger();
        ChatModel model = prompt ->
        {
            if (calls.incrementAndGet() == 1)
            {
                throw new RuntimeException("Error reading response", new InterruptedIOException("timeout"));
            }
            return response("OK", 3, 2);
        };
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.OPENAI);
        properties.setApiKey("test-key");
        ProviderFailureTranslator translator = new SpringAiProviderIntegration(new DefaultResourceLoader())
                .create("openai", properties)
                .failureTranslator();
        ProviderConnectionRuntime runtime = new ProviderConnectionRuntime(
                model,
                AiDriver.OPENAI,
                AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                new ProviderRetryPolicy(true, 2, java.time.Duration.ZERO, 2.0d,
                        java.time.Duration.ZERO, 0.0d),
                translator);
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, stateService,
                        new ModelUsageExtractor(), usageService))
                .build();

        String content = SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext()))
                .call()
                .content());

        assertThat(content).isEqualTo("OK");
        assertThat(calls).hasValue(2);
        List<TraceRecord> records = records(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT))
                .hasSize(2);
        assertThat(records.stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_ATTEMPT_FAILED))
                .singleElement()
                .satisfies(record -> assertThat(record.metadata())
                        .containsEntry("failureClassification", "TRANSIENT")
                        .containsEntry("failureCategory", "TIMEOUT")
                        .containsEntry("retryDecision", "RETRY")
                        .containsEntry("providerAttemptNumber", 1));
    }

    @Test
    void exhaustedProviderRetriesRetainExactAttemptQuotaMetricAndTerminalFacts()
    {
        SimpleMeterRegistry meterRegistry = new SimpleMeterRegistry();
        DefaultSessionUsageService usageService = new DefaultSessionUsageService(
                new LoomspanProperties().getSession().getQuotas(),
                new MicrometerUsageMetricsRecorder(meterRegistry));
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("provider-exhaustion", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        AtomicInteger calls = new AtomicInteger();
        IllegalStateException terminal = new IllegalStateException("provider remained unavailable");
        ChatModel model = prompt ->
        {
            calls.incrementAndGet();
            throw terminal;
        };
        ProviderFailureDetails transientFailure = new ProviderFailureDetails(
                ProviderFailureClassification.TRANSIENT,
                ProviderFailureCategory.SERVER_ERROR,
                503,
                null,
                null,
                null,
                "Provider temporarily unavailable",
                List.of());
        ProviderConnectionRuntime runtime = new ProviderConnectionRuntime(
                model,
                AiDriver.OPENAI,
                AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                new ProviderRetryPolicy(true, 3, java.time.Duration.ZERO, 2.0d,
                        java.time.Duration.ZERO, 0.0d),
                ignored -> transientFailure);
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, stateService,
                        new ModelUsageExtractor(), usageService))
                .build();

        assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt()
                .user("user")
                .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext()))
                .call()
                .content()))
                .isSameAs(terminal);

        assertThat(calls).hasValue(3);
        assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(3);
        assertThat(session.getSessionUsage().orElseThrow().modelCalls()).isZero();
        List<TraceRecord> records = records(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT))
                .hasSize(3);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED))
                .isEmpty();
        List<TraceRecord> failures = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_ATTEMPT_FAILED)
                .toList();
        assertThat(failures).hasSize(3);
        assertThat(failures).extracting(record -> record.metadata().get("providerAttemptNumber"))
                .containsExactly(1, 2, 3);
        assertThat(failures).extracting(record -> record.metadata().get("retryDecision"))
                .containsExactly("RETRY", "RETRY", "ATTEMPTS_EXHAUSTED");
        assertThat(failures).extracting(record -> record.metadata().get("retrySequenceId"))
                .containsOnly(failures.getFirst().metadata().get("retrySequenceId"));
        assertThat(meterRegistry.get("loomspan.provider.attempts")
                .tag("skill", "test.skill")
                .tag("connection", "test-connection")
                .tag("driver", "openai")
                .tag("outcome", "failed")
                .tag("category", "server_error")
                .meters()).hasSize(2);
        assertThat(meterRegistry.get("loomspan.provider.attempts")
                .tag("decision", "retry").counter().count()).isEqualTo(2.0d);
        assertThat(meterRegistry.get("loomspan.provider.attempts")
                .tag("decision", "attempts_exhausted").counter().count()).isEqualTo(1.0d);
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
                        new ProviderAttemptCallAdvisor(runtime(model), stateService, new ModelUsageExtractor(), usageService))
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

    @Test
    void interruptionDuringBackoffPreservesInterruptAndCreatesNoPhantomAttempt() throws Exception
    {
        DefaultSessionUsageService usageService = usageService();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("retry-interrupted", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        ModelTraceContext traceContext = traceContext();
        CountDownLatch firstCall = new CountDownLatch(1);
        AtomicInteger calls = new AtomicInteger();
        ChatModel model = prompt ->
        {
            calls.incrementAndGet();
            firstCall.countDown();
            throw new IllegalStateException("temporary provider failure");
        };
        ProviderFailureDetails transientFailure = new ProviderFailureDetails(
                ProviderFailureClassification.TRANSIENT, ProviderFailureCategory.SERVER_ERROR,
                503, null, null, null, null, List.of());
        ProviderConnectionRuntime runtime = new ProviderConnectionRuntime(model, AiDriver.OPENAI,
                AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                new ProviderRetryPolicy(true, 3, java.time.Duration.ofSeconds(30), 2.0d,
                        java.time.Duration.ofSeconds(30), 0.0d), ignored -> transientFailure);
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, stateService,
                        new ModelUsageExtractor(), usageService)).build();
        AtomicReference<Throwable> observed = new AtomicReference<>();
        AtomicBoolean interruptPreserved = new AtomicBoolean();
        Thread worker = new Thread(() ->
        {
            try
            {
                SessionContextRunner.callWithSession(session, () -> client.prompt().user("user")
                        .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                        .call().content());
            }
            catch (Throwable failure)
            {
                observed.set(failure);
                interruptPreserved.set(Thread.currentThread().isInterrupted());
            }
        });
        worker.start();
        assertThat(firstCall.await(5, TimeUnit.SECONDS)).isTrue();
        worker.interrupt();
        worker.join(5_000);

        assertThat(worker.isAlive()).isFalse();
        assertThat(observed.get()).isInstanceOf(CancellationException.class);
        assertThat(interruptPreserved).isTrue();
        assertThat(calls).hasValue(1);
        assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(1);
        assertThat(records(session).stream().filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT))
                .hasSize(1);
        assertThat(records(session).stream().filter(record -> record.recordType() == TraceRecordType.MODEL_ATTEMPT_FAILED))
                .singleElement().satisfies(record -> assertThat(record.metadata()).containsEntry("retryDecision", "RETRY"));
    }

    @Test
    void interruptionBeforeSendCreatesNoAttempt()
    {
        DefaultSessionUsageService usageService = usageService();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        LoomspanSession session = TestLoomspanSessions.withId("retry-pre-interrupted", "test.entry", 4);
        stateService.openMissionFrame(session, "test.skill", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        AtomicInteger calls = new AtomicInteger();
        ChatModel model = prompt ->
        {
            calls.incrementAndGet();
            return response("unexpected", 1, 1);
        };
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime(model), stateService,
                        new ModelUsageExtractor(), usageService)).build();

        Thread.currentThread().interrupt();
        try
        {
            assertThatThrownBy(() -> SessionContextRunner.callWithSession(session, () -> client.prompt().user("user")
                    .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext()))
                    .call().content())).isInstanceOf(CancellationException.class);
            assertThat(Thread.currentThread().isInterrupted()).isTrue();
        }
        finally
        {
            Thread.interrupted();
        }
        assertThat(calls).hasValue(0);
        assertThat(session.getSessionUsage()).isEmpty();
        assertThat(records(session)).noneMatch(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_SENT);
    }

    @Test
    void openRouterRetryableErrorCompletionRecoversWithTwoVisibleEndpointCalls() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                    {"id":"error-1","object":"chat.completion","created":1,"model":"routed-model",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"unsafe partial"},
                       "finish_reason":"error","error":{"message":"overloaded","code":"E_OVERLOAD",
                       "metadata":{"error_type":"provider_overloaded"}}}]}
                    """));
            server.enqueue(new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                    {"id":"success-2","object":"chat.completion","created":2,"model":"routed-model",
                     "choices":[{"index":0,"message":{"role":"assistant","content":"recovered"},"finish_reason":"stop"}],
                     "usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}
                    """));
            LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
            properties.setDriver(AiDriver.OPENAI);
            properties.setApiKey("gateway-key");
            properties.setBaseUrl(server.url("/v1").toString());
            LoomspanProperties.OpenAiOptions openAi = new LoomspanProperties.OpenAiOptions();
            openAi.setCompatibilityProfile(LoomspanProperties.OpenAiCompatibilityProfile.OPENROUTER);
            properties.setOpenai(openAi);
            properties.getProviderRetry().setInitialBackoff(java.time.Duration.ZERO);
            properties.getProviderRetry().setMaxBackoff(java.time.Duration.ZERO);
            properties.getProviderRetry().setJitter(0.0d);
            ProviderConnectionRuntime runtime = new SpringAiProviderIntegration(new DefaultResourceLoader())
                    .create("openrouter", properties);
            DefaultSessionUsageService usageService = usageService();
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
            LoomspanSession session = TestLoomspanSessions.withId("openrouter-recovery", "test.entry", 4);
            stateService.openMissionFrame(session, "test.skill", Map.of());
            stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
            ModelTraceContext traceContext = traceContext();
            ChatClient client = ChatClient.builder(runtime.chatModel())
                    .defaultAdvisors(new ProviderAttemptCallAdvisor(runtime, stateService,
                            new ModelUsageExtractor(), usageService)).build();

            String content = SessionContextRunner.callWithSession(session, () -> client.prompt()
                    .user("hello")
                    .options(OpenAiChatOptions.builder().model("routed-model"))
                    .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext))
                    .call().content());

            assertThat(content).isEqualTo("recovered");
            assertThat(server.getRequestCount()).isEqualTo(2);
            assertThat(session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(2);
            assertThat(records(session).stream().filter(record -> record.recordType() == TraceRecordType.MODEL_ATTEMPT_FAILED))
                    .singleElement().satisfies(record -> assertThat(record.data().toString()).contains("unsafe partial"));
            assertThat(records(session).stream().filter(record -> record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED))
                    .singleElement().satisfies(record -> assertThat(record.metadata())
                            .containsEntry("attemptReason", "PROVIDER_RETRY")
                            .containsEntry("providerAttemptNumber", 2));
            assertThat(records(session)).noneMatch(record -> record.recordType() == TraceRecordType.ERROR_RECORDED);
        }
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

    private static ProviderConnectionRuntime runtime(ChatModel model)
    {
        LoomspanProperties.ProviderRetryProperties retry = new LoomspanProperties.ProviderRetryProperties();
        retry.setEnabled(false);
        return new ProviderConnectionRuntime(model, AiDriver.OPENAI, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                ProviderRetryPolicy.from(retry), ignored -> ProviderFailureDetails.unknown());
    }

    private static ProviderConnectionRuntime retryingRuntime(ChatModel model, ProviderFailureDetails details)
    {
        return new ProviderConnectionRuntime(model, AiDriver.OPENAI, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                new ProviderRetryPolicy(true, 2, java.time.Duration.ZERO, 2.0d, java.time.Duration.ZERO, 0.0d),
                ignored -> details);
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

    private static MockResponse openAiToolCall(String callId, String value)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"chatcmpl-tool","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":null,
                   "tool_calls":[{"id":"%s","type":"function","function":{"name":"lookup","arguments":"{\\\"value\\\":\\\"%s\\\"}"}}]},
                   "finish_reason":"tool_calls"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(callId, value));
    }

    private static MockResponse openAiText(String text)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"chatcmpl-text","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(text));
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
