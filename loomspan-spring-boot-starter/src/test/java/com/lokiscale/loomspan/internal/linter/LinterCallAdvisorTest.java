package com.lokiscale.loomspan.internal.linter;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.chat.prompt.Prompt;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.regex.Pattern;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LinterCallAdvisorTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void returnsPassingResponseWithoutRetry() {
        LinterCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("OK: valid"));

        ChatClientResponse response = advisor.adviseCall(request("Write YAML"), chain);

        assertThat(chain.requests).hasSize(1);
        assertThat(text(response)).isEqualTo("OK: valid");
        assertThat(response.context()).containsKey(LinterCallAdvisor.CONTEXT_KEY);
        assertThat((LinterOutcome) response.context().get(LinterCallAdvisor.CONTEXT_KEY))
                .extracting(LinterOutcome::status, LinterOutcome::retryCount)
                .containsExactly(LinterOutcomeStatus.PASSED, 0);
    }

    @Test
    void retriesWithCorrectiveHintAfterFailedValidation() {
        LinterCallAdvisor advisor = advisor(2);
        RecordingChain chain = new RecordingChain(List.of("invalid", "OK: corrected"));

        ChatClientResponse response = advisor.adviseCall(request("Write YAML"), chain);

        assertThat(chain.requests).hasSize(2);
        assertThat(chain.requests.get(1).prompt().getUserMessage().getText())
                .isEqualTo("Write YAML");
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .contains("Linter validation failed")
                .contains("Return fenced YAML only.");
        assertThat(chain.copyInvocations()).isEqualTo(2);
        assertThat((LinterOutcome) response.context().get(LinterCallAdvisor.CONTEXT_KEY))
                .extracting(LinterOutcome::status, LinterOutcome::retryCount, LinterOutcome::attempt)
                .containsExactly(LinterOutcomeStatus.PASSED, 1, 2);
    }

    @Test
    void retryPromptPreservesExistingSkillPrompt() {
        LinterCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("invalid", "OK: corrected"));

        advisor.adviseCall(request("Write YAML", "SKILL_PROMPT_SENTINEL\n\nBase instructions."), chain);

        assertThat(chain.requests).hasSize(2);
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .startsWith("SKILL_PROMPT_SENTINEL")
                .contains("Linter validation failed")
                .contains("Return fenced YAML only.");
    }

    @Test
    void stopsRetryingWhenMaxRetriesAreExhausted() {
        LinterCallAdvisor advisor = advisor(2);
        RecordingChain chain = new RecordingChain(List.of("bad-1", "bad-2", "bad-3"));

        ChatClientResponse response = advisor.adviseCall(request("Write YAML"), chain);

        assertThat(chain.requests).hasSize(3);
        assertThat(text(response)).isEqualTo("bad-3");
        assertThat((LinterOutcome) response.context().get(LinterCallAdvisor.CONTEXT_KEY))
                .extracting(LinterOutcome::status, LinterOutcome::retryCount, LinterOutcome::attempt)
                .containsExactly(LinterOutcomeStatus.EXHAUSTED, 2, 3);
    }

    @Test
    void recordsEachOutcomeThroughRecorderBoundary() {
        RecordingOutcomeRecorder recorder = new RecordingOutcomeRecorder();
        LinterCallAdvisor advisor = advisor(2, recorder);
        RecordingChain chain = new RecordingChain(List.of("bad", "OK: corrected"));

        ChatClientResponse response = advisor.adviseCall(request("Write YAML"), chain);

        assertThat(text(response)).isEqualTo("OK: corrected");
        assertThat(recorder.outcomes)
                .extracting(LinterOutcome::status, LinterOutcome::retryCount, LinterOutcome::attempt)
                .containsExactly(
                        org.assertj.core.groups.Tuple.tuple(LinterOutcomeStatus.RETRYING, 0, 1),
                        org.assertj.core.groups.Tuple.tuple(LinterOutcomeStatus.PASSED, 1, 2));
    }

    @Test
    void recordsObservableOutcomeOnBoundSession() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LinterCallAdvisor advisor = advisor(1, outcome ->
                stateService.recordLinterOutcome(LoomspanSession.getCurrentSession(), outcome));
        RecordingChain chain = new RecordingChain(List.of("bad", "OK: corrected"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        runner.callWithNewSession("test.entry", session -> {
            ChatClientResponse response = advisor.adviseCall(request("Write YAML"), chain);

            assertThat(text(response)).isEqualTo("OK: corrected");
            assertThat(session.getLastLinterOutcome()).isPresent();
            assertThat(session.getLastLinterOutcome().orElseThrow())
                    .extracting(LinterOutcome::status, LinterOutcome::retryCount)
                    .containsExactly(LinterOutcomeStatus.PASSED, 1);
            assertThat(session.getJournalSnapshot())
                    .extracting(JournalEntry::type)
                    .containsExactly(JournalEntryType.LINTER, JournalEntryType.LINTER);
            return session;
        });
    }

    @Test
    void recordsAdvisorMutationsOnTheActiveFrame() throws Exception {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LinterCallAdvisor advisor = advisor(1, outcome ->
                stateService.recordLinterOutcome(LoomspanSession.getCurrentSession(), outcome));
        RecordingChain chain = new RecordingChain(List.of("bad", "OK: corrected"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        runner.callWithNewSession("test.entry", session -> {
            var frame = stateService.openFrame(session, TraceFrameType.MODEL_CALL, "lintedSkill#model", Map.of());

            advisor.adviseCall(request("Write YAML"), chain);

            List<TraceRecord> records = readRecords(session);
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED)
                    .allMatch(record -> frame.frameId().equals(record.frameId()) && "lintedSkill#model".equals(record.route())))
                    .isTrue();
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ADVISOR_RESPONSE_MUTATION_RECORDED)
                    .allMatch(record -> frame.frameId().equals(record.frameId()) && "lintedSkill#model".equals(record.route())))
                    .isTrue();

            stateService.closeFrame(session, frame, Map.of("status", "completed"));
            return session;
        });
    }

    @Test
    void rethrowsManagedSessionRecorderFailures() {
        LinterCallAdvisor advisor = advisor(0, outcome -> {
            throw new IllegalStateException("boom");
        });
        RecordingChain chain = new RecordingChain(List.of("bad-1"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> advisor.adviseCall(request("Write YAML"), chain)))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("boom");
    }

    private static LinterCallAdvisor advisor(int maxRetries) {
        return advisor(maxRetries, outcome -> {
        });
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static LinterCallAdvisor advisor(int maxRetries, LinterOutcomeRecorder outcomeRecorder) {
        return new LinterCallAdvisor(
                "lintedSkill",
                "regex",
                Pattern.compile("^OK:.*$"),
                "Return fenced YAML only.",
                maxRetries,
                outcomeRecorder);
    }

    private static ChatClientRequest request(String text) {
        return new ChatClientRequest(new Prompt(text), Map.of());
    }

    private static ChatClientRequest request(String userText, String systemText) {
        return new ChatClientRequest(new Prompt(List.of(
                new org.springframework.ai.chat.messages.SystemMessage(systemText),
                new org.springframework.ai.chat.messages.UserMessage(userText))), Map.of());
    }

    private static String text(ChatClientResponse response) {
        return response.chatResponse().getResult().getOutput().getText();
    }

    private static final class RecordingChain implements CallAdvisorChain {

        private final List<String> responses;
        private final List<ChatClientRequest> requests;
        private final AtomicInteger index;
        private final AtomicInteger copyInvocations;
        private final AtomicBoolean consumed;

        private RecordingChain(List<String> responses) {
            this(responses, new ArrayList<>(), new AtomicInteger(), new AtomicInteger(), new AtomicBoolean());
        }

        private RecordingChain(List<String> responses,
                               List<ChatClientRequest> requests,
                               AtomicInteger index,
                               AtomicInteger copyInvocations,
                               AtomicBoolean consumed) {
            this.responses = responses;
            this.requests = requests;
            this.index = index;
            this.copyInvocations = copyInvocations;
            this.consumed = consumed;
        }

        @Override
        public ChatClientResponse nextCall(ChatClientRequest chatClientRequest) {
            if (!consumed.compareAndSet(false, true)) {
                throw new IllegalStateException("No CallAdvisors available to execute");
            }
            requests.add(chatClientRequest.copy());
            String responseText = responses.get(Math.min(index.getAndIncrement(), responses.size() - 1));
            return ChatClientResponse.builder()
                    .chatResponse(new ChatResponse(List.of(new Generation(new AssistantMessage(responseText)))))
                    .build();
        }

        @Override
        public List<CallAdvisor> getCallAdvisors() {
            return List.of();
        }

        @Override
        public CallAdvisorChain copy(CallAdvisor after) {
            copyInvocations.incrementAndGet();
            return new RecordingChain(responses, requests, index, copyInvocations, new AtomicBoolean());
        }

        private int copyInvocations() {
            return copyInvocations.get();
        }
    }

    private static final class RecordingOutcomeRecorder implements LinterOutcomeRecorder {

        private final List<LinterOutcome> outcomes = new ArrayList<>();

        @Override
        public void record(LinterOutcome outcome) {
            outcomes.add(outcome);
        }
    }
}
