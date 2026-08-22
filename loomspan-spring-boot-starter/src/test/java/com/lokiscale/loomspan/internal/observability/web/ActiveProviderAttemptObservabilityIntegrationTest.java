package com.lokiscale.loomspan.internal.observability.web;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.chat.ProviderAttemptCallAdvisor;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.SessionContextRunner;
import com.lokiscale.loomspan.internal.core.TestLoomspanSessions;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos;
import com.lokiscale.loomspan.internal.provider.AttemptOwnership;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.provider.ProviderFailureCategory;
import com.lokiscale.loomspan.internal.provider.ProviderFailureClassification;
import com.lokiscale.loomspan.internal.provider.ProviderFailureDetails;
import com.lokiscale.loomspan.internal.provider.ProviderRetryPolicy;
import com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot;
import com.lokiscale.loomspan.internal.runtime.observation.DefaultExecutionObservationHandleFactory;
import com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivityKind;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.DefaultSessionUsageService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpUsageMetricsRecorder;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.metadata.ChatResponseMetadata;
import org.springframework.ai.chat.metadata.DefaultUsage;
import org.springframework.ai.chat.model.ChatModel;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;

class ActiveProviderAttemptObservabilityIntegrationTest
{
    private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-08-21T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void providerRequestIsVisibleInActiveRestBeforeResponseAccounting() throws Exception
    {
        LoomspanProperties properties = new LoomspanProperties();
        DefaultSessionUsageService usageService = new DefaultSessionUsageService(
                properties.getSession().getQuotas(), new NoOpUsageMetricsRecorder());
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK, usageService);
        DefaultExecutionObservationHandleFactory observation = new DefaultExecutionObservationHandleFactory();
        LoomspanSession session = TestLoomspanSessions.withObservation(
                "active-provider-attempt", "test.entry", 4, CLOCK, observation);
        stateService.openMissionFrame(session, "test.entry", Map.of());
        stateService.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());

        CountDownLatch firstEntered = new CountDownLatch(1);
        CountDownLatch releaseFirst = new CountDownLatch(1);
        CountDownLatch secondEntered = new CountDownLatch(1);
        CountDownLatch releaseSecond = new CountDownLatch(1);
        AtomicInteger calls = new AtomicInteger();
        ChatModel model = prompt ->
        {
            int call = calls.incrementAndGet();
            if (call == 1)
            {
                firstEntered.countDown();
                await(releaseFirst);
                throw new IllegalStateException("temporary provider failure");
            }
            secondEntered.countDown();
            await(releaseSecond);
            return response("OK");
        };
        ProviderFailureDetails transientFailure = new ProviderFailureDetails(
                ProviderFailureClassification.TRANSIENT, ProviderFailureCategory.SERVER_ERROR,
                503, null, null, null, "Provider temporarily unavailable", List.of());
        ProviderConnectionRuntime runtime = new ProviderConnectionRuntime(
                model, AiDriver.OPENAI, AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP,
                new ProviderRetryPolicy(true, 2, Duration.ZERO, 2.0d, Duration.ZERO, 0.0d),
                ignored -> transientFailure);
        ChatClient client = ChatClient.builder(model)
                .defaultAdvisors(new ProviderAttemptCallAdvisor(
                        runtime, stateService, new ModelUsageExtractor(), usageService))
                .build();
        AtomicReference<Throwable> failure = new AtomicReference<>();
        Thread worker = new Thread(() ->
        {
            try
            {
                SessionContextRunner.callWithSession(session, () -> client.prompt()
                        .user("user")
                        .advisors(spec -> spec.param(ModelTraceContext.REQUEST_CONTEXT_KEY, traceContext()))
                        .call()
                        .content());
            }
            catch (Throwable throwable)
            {
                failure.set(throwable);
            }
        });

        try
        {
            worker.start();
            assertThat(firstEntered.await(5, TimeUnit.SECONDS)).isTrue();
            assertSerializedUsage(observation, properties, 1, 0);
            assertThat(observation.replayBuffer().replayAfter(0, 100).activities())
                    .anyMatch(activity -> activity.kind() == ExecutionActivityKind.MODEL_REQUEST_SENT);

            releaseFirst.countDown();
            assertThat(secondEntered.await(5, TimeUnit.SECONDS)).isTrue();
            assertSerializedUsage(observation, properties, 2, 0);

            releaseSecond.countDown();
            worker.join(5_000);
            assertThat(worker.isAlive()).isFalse();
            assertThat(failure.get()).isNull();
            assertSerializedUsage(observation, properties, 2, 1);
        }
        finally
        {
            releaseFirst.countDown();
            releaseSecond.countDown();
            worker.join(5_000);
        }
    }

    private static void assertSerializedUsage(DefaultExecutionObservationHandleFactory observation,
                                              LoomspanProperties properties,
                                              int providerAttempts,
                                              int modelCalls) throws Exception
    {
        ActiveExecutionSnapshot snapshot = observation.registry().find("active-provider-attempt").orElseThrow();
        ObservabilityDtos.ActiveExecution mapped = new ObservabilityDtoMapper().active(
                snapshot, CLOCK.instant(), properties.getSession().getQuotas());
        ObservabilityJsonCodec codec = new ObservabilityJsonCodec();
        ObservabilityDtos.ActiveExecution serialized = codec.read(
                codec.write(mapped), ObservabilityDtos.ActiveExecution.class);

        assertThat(serialized.usage().providerAttempts()).isEqualTo(providerAttempts);
        assertThat(serialized.usage().modelCalls()).isEqualTo(modelCalls);
        if (modelCalls == 0)
        {
            assertThat(serialized.usage().promptUnits()).isZero();
            assertThat(serialized.usage().completionUnits()).isZero();
            assertThat(serialized.usage().usageUnits()).isZero();
            assertThat(serialized.usage().exactModelResponses()).isZero();
            assertThat(serialized.usage().heuristicModelResponses()).isZero();
            assertThat(serialized.usage().unavailableModelResponses()).isZero();
        }
    }

    private static void await(CountDownLatch latch)
    {
        try
        {
            if (!latch.await(5, TimeUnit.SECONDS))
            {
                throw new IllegalStateException("test latch timed out");
            }
        }
        catch (InterruptedException ex)
        {
            Thread.currentThread().interrupt();
            throw new IllegalStateException("test worker interrupted", ex);
        }
    }

    private static ModelTraceContext traceContext()
    {
        return new ModelTraceContext(
                new ModelExecutionIdentity("test-model", "test-connection", AiDriver.OPENAI, "provider/model"),
                "test.skill", "mission");
    }

    private static ChatResponse response(String text)
    {
        return new ChatResponse(
                List.of(new Generation(new AssistantMessage(text))),
                ChatResponseMetadata.builder().usage(new DefaultUsage(3, 2, 5)).build());
    }
}
