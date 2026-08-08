package com.lokiscale.loomspan.internal.runtime.observation;

import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.TracePersistencePolicy;
import org.junit.jupiter.api.RepeatedTest;

import java.time.Clock;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;

class ExecutionObservationConcurrencyTest
{
    private static final int SESSION_COUNT = 128;

    @RepeatedTest(10)
    void representsEveryBlockedLiveSessionWithoutSamplingAndRemovesAllAfterRelease() throws Exception
    {
        DefaultExecutionObservationHandleFactory factory = new DefaultExecutionObservationHandleFactory();
        LoomspanSessionRunner runner = new LoomspanSessionRunner(
                4, TracePersistencePolicy.NEVER, Clock.systemUTC(), factory);
        CountDownLatch ready = new CountDownLatch(SESSION_COUNT);
        CountDownLatch release = new CountDownLatch(1);
        List<Future<String>> futures = new ArrayList<>(SESSION_COUNT);

        try (var executor = Executors.newVirtualThreadPerTaskExecutor())
        {
            for (int index = 0; index < SESSION_COUNT; index++)
            {
                futures.add(executor.submit(() -> runner.callWithNewSession("test.entry", session ->
                {
                    ready.countDown();
                    try
                    {
                        if (!release.await(30, TimeUnit.SECONDS))
                        {
                            throw new IllegalStateException("release timed out");
                        }
                    }
                    catch (InterruptedException ex)
                    {
                        Thread.currentThread().interrupt();
                        throw new IllegalStateException("interrupted", ex);
                    }
                    return session.getSessionId();
                })));
            }

            assertThat(ready.await(30, TimeUnit.SECONDS)).isTrue();
            assertThat(factory.registry().activeCount()).isEqualTo(SESSION_COUNT);
            assertThat(factory.registry().newestFirst(0, 0, SESSION_COUNT))
                    .extracting(ActiveExecutionSnapshot::sessionId)
                    .doesNotHaveDuplicates()
                    .hasSize(SESSION_COUNT);

            release.countDown();
            for (Future<String> future : futures)
            {
                assertThat(future.get()).isNotBlank();
            }
        }

        assertThat(factory.registry().activeCount()).isZero();
        List<ExecutionActivity> activities = factory.replayBuffer().replayAfter(0, 10_000).activities();
        assertThat(activities).hasSize(SESSION_COUNT * 2);
        assertThat(activities).extracting(ExecutionActivity::deliveryCursor)
                .isSorted()
                .doesNotHaveDuplicates();
        Map<String, List<Long>> sequencesByTrace = activities.stream().collect(Collectors.groupingBy(
                ExecutionActivity::traceId,
                Collectors.mapping(ExecutionActivity::canonicalSequence, Collectors.toList())));
        assertThat(sequencesByTrace).hasSize(SESSION_COUNT);
        assertThat(sequencesByTrace.values()).allSatisfy(sequences ->
        {
            assertThat(sequences).doesNotContainNull();
            assertThat(new HashSet<>(sequences)).hasSameSizeAs(sequences);
            assertThat(sequences).isSorted();
        });
    }
}
