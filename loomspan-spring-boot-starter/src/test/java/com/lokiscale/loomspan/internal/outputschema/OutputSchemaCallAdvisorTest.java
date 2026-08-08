package com.lokiscale.loomspan.internal.outputschema;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.boot.test.system.CapturedOutput;
import org.springframework.boot.test.system.OutputCaptureExtension;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@ExtendWith(OutputCaptureExtension.class)
class OutputSchemaCallAdvisorTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void returnsPassingResponseWithoutRetryWhenJsonMatchesSchema() {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

        assertThat(chain.requests).hasSize(1);
        assertThat(text(response)).isEqualTo("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}");
        assertThat(chain.requests.getFirst().prompt().getSystemMessage().getText())
                .contains("Return JSON only.")
                .contains("vendorName")
                .doesNotContain("invoiceParser")
                .doesNotContain("\"evidence\"");
        assertThat((OutputSchemaOutcome) response.context().get(OutputSchemaCallAdvisor.CONTEXT_KEY))
                .extracting(OutputSchemaOutcome::status, OutputSchemaOutcome::retryCount)
                .containsExactly(OutputSchemaOutcomeStatus.PASSED, 0);
    }

    @Test
    void retriesWithCorrectiveHintAfterInvalidJson() {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("not-json", "{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

        assertThat(text(response)).isEqualTo("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}");
        assertThat(chain.requests).hasSize(2);
        assertThat(chain.requests.get(1).prompt().getUserMessage().getText()).isEqualTo("Extract invoice");
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .contains("Output schema validation failed")
                .contains("Response is not valid JSON.");
        assertThat(chain.copyInvocations()).isEqualTo(2);
    }

    @Test
    void retriesWhenModelReturnsBlankResponse() {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("", "{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

        assertThat(text(response)).isEqualTo("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}");
        assertThat(chain.requests).hasSize(2);
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .contains("Output schema validation failed")
                .contains("Response is not valid JSON.");
    }

    @Test
    void retriesWithCanonicalIssuesAfterSchemaMismatch() {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of(
                "{\"companyName\":\"Acme\",\"totalAmount\":\"42.5\",\"evidence\":\"invoiceParser\"}",
                "{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

        assertThat(text(response)).isEqualTo("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}");
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .contains("vendorName: missing required field 'vendorName'")
                .contains("companyName: unknown field 'companyName'")
                .contains("evidence: unknown field 'evidence'")
                .contains("totalAmount: should be a number");
    }

    @Test
    void includesFormatHintsInPromptGuidance() {
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                schemaWithDateFormat(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                0,
                outcome -> {
                });
        RecordingChain chain = new RecordingChain(List.of("{\"invoiceDate\":\"2026-03-15\"}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

        assertThat(text(response)).isEqualTo("{\"invoiceDate\":\"2026-03-15\"}");
        assertThat(chain.requests).hasSize(1);
        assertThat(chain.requests.getFirst().prompt().getSystemMessage().getText())
                .contains("invoiceDate")
                .contains("format date");
    }

    @Test
    void acceptsNullableSchemaFieldsAndMentionsNullabilityInPromptGuidance() {
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                nullableSchema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                0,
                outcome -> {
                });
        RecordingChain chain = new RecordingChain(List.of("{\"driverName\":null}"));

        ChatClientResponse response = advisor.adviseCall(request("Extract ticket"), chain);

        assertThat(text(response)).isEqualTo("{\"driverName\":null}");
        assertThat(chain.requests).hasSize(1);
        assertThat(chain.requests.getFirst().prompt().getSystemMessage().getText())
                .contains("driverName")
                .contains("string or null");
    }

    @Test
    void rejectsAmbiguousCaseInsensitiveKeys() {
        OutputSchemaCallAdvisor advisor = advisor(0);
        RecordingChain chain = new RecordingChain(List.of("{\"vendorName\":\"Acme\",\"VendorName\":\"Other\",\"totalAmount\":42.5}"));

        assertThatThrownBy(() -> advisor.adviseCall(request("Extract invoice"), chain))
                .isInstanceOf(LoomspanOutputSchemaValidationException.class)
                .hasMessageContaining("SCHEMA_VALIDATION_FAILED");
    }

    @Test
    void throwsLoomspanOutputSchemaValidationExceptionWhenRetriesExhausted() {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("bad-json", "still bad"));

        assertThatThrownBy(() -> advisor.adviseCall(request("Extract invoice"), chain))
                .isInstanceOf(LoomspanOutputSchemaValidationException.class)
                .satisfies(ex -> {
                    LoomspanOutputSchemaValidationException failure = (LoomspanOutputSchemaValidationException) ex;
                    assertThat(failure.getSkillName()).isEqualTo("outputSchemaSkill");
                    assertThat(failure.getRawOutput()).isEqualTo("still bad");
                    assertThat(failure.getAttemptCount()).isEqualTo(2);
                    assertThat(failure.getMaxRetries()).isEqualTo(1);
                    assertThat(failure.getFailureMode()).isEqualTo(OutputSchemaFailureMode.INVALID_JSON);
                    assertThat(failure.getValidationIssues()).isNotEmpty();
                });
    }

    @Test
    void recordsObservableOutcomeOnBoundSession() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                schema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                1,
                outcome -> stateService.recordOutputSchemaOutcome(LoomspanSession.getCurrentSession(), outcome));
        RecordingChain chain = new RecordingChain(List.of("bad-json", "{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        runner.callWithNewSession("test.entry", session -> {
            ChatClientResponse response = advisor.adviseCall(request("Extract invoice"), chain);

            assertThat(text(response)).isEqualTo("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}");
            assertThat(session.getLastOutputSchemaOutcome()).isPresent();
            assertThat(session.getLastOutputSchemaOutcome().orElseThrow())
                    .extracting(OutputSchemaOutcome::status, OutputSchemaOutcome::retryCount, OutputSchemaOutcome::failureMode)
                    .containsExactly(OutputSchemaOutcomeStatus.PASSED, 1, null);
            assertThat(session.getJournalSnapshot())
                    .extracting(JournalEntry::type)
                    .containsExactly(JournalEntryType.OUTPUT_SCHEMA, JournalEntryType.OUTPUT_SCHEMA);
            return session;
        });
    }

    @Test
    void truncatesRecordedOutcomeIssuesToBoundedList() {
        List<OutputSchemaOutcome> recordedOutcomes = new ArrayList<>();
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                wideSchema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                0,
                recordedOutcomes::add);
        RecordingChain chain = new RecordingChain(List.of("{}"));

        assertThatThrownBy(() -> advisor.adviseCall(request("Extract invoice"), chain))
                .isInstanceOf(LoomspanOutputSchemaValidationException.class)
                .satisfies(ex -> {
                    LoomspanOutputSchemaValidationException failure = (LoomspanOutputSchemaValidationException) ex;
                    assertThat(failure.getValidationIssues()).hasSize(6);
                });

        assertThat(recordedOutcomes).hasSize(1);
        assertThat(recordedOutcomes.getFirst().status()).isEqualTo(OutputSchemaOutcomeStatus.EXHAUSTED);
        assertThat(recordedOutcomes.getFirst().issues()).hasSize(4);
    }

    @Test
    void logsRuntimeSchemaFailures(CapturedOutput output) {
        OutputSchemaCallAdvisor advisor = advisor(1);
        RecordingChain chain = new RecordingChain(List.of("bad-json", "still bad"));

        assertThatThrownBy(() -> advisor.adviseCall(request("Extract invoice"), chain))
                .isInstanceOf(LoomspanOutputSchemaValidationException.class);

        assertThat(output.getOut())
                .contains("Output schema validation retry for skill 'outputSchemaSkill'")
                .contains("Output schema validation exhausted for skill 'outputSchemaSkill'")
                .contains("failureMode=INVALID_JSON");
    }

    @Test
    void recordsAdvisorMutationsOnTheActiveFrame() throws Exception {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                schema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                1,
                outcome -> stateService.recordOutputSchemaOutcome(LoomspanSession.getCurrentSession(), outcome));
        RecordingChain chain = new RecordingChain(List.of("bad-json", "{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        runner.callWithNewSession("test.entry", session -> {
            var frame = stateService.openFrame(session, TraceFrameType.MODEL_CALL, "outputSchemaSkill#model", Map.of());

            advisor.adviseCall(request("Extract invoice"), chain);

            List<TraceRecord> records = readRecords(session);
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ADVISOR_REQUEST_MUTATION_RECORDED)
                    .allMatch(record -> frame.frameId().equals(record.frameId()) && "outputSchemaSkill#model".equals(record.route())))
                    .isTrue();
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ADVISOR_RESPONSE_MUTATION_RECORDED)
                    .allMatch(record -> frame.frameId().equals(record.frameId()) && "outputSchemaSkill#model".equals(record.route())))
                    .isTrue();

            stateService.closeFrame(session, frame, Map.of("status", "completed"));
            return session;
        });
    }

    @Test
    void rethrowsManagedSessionRecorderFailures() {
        OutputSchemaCallAdvisor advisor = new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                schema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                0,
                outcome -> {
                    throw new IllegalStateException("boom");
                });
        RecordingChain chain = new RecordingChain(List.of("{\"vendorName\":\"Acme\",\"totalAmount\":42.5}"));
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session -> advisor.adviseCall(request("Extract invoice"), chain)))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("boom");
    }

    private static OutputSchemaCallAdvisor advisor(int maxRetries) {
        return new OutputSchemaCallAdvisor(
                "outputSchemaSkill",
                schema(),
                new OutputSchemaValidator(),
                new OutputSchemaPromptAugmentor(),
                maxRetries,
                outcome -> {
                });
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static YamlSkillManifest.OutputSchemaManifest schema() {
        YamlSkillManifest.OutputSchemaManifest root = new YamlSkillManifest.OutputSchemaManifest();
        root.setType("object");

        YamlSkillManifest.OutputSchemaManifest vendorName = new YamlSkillManifest.OutputSchemaManifest();
        vendorName.setType("string");
        vendorName.setEvidence("invoiceParser");

        YamlSkillManifest.OutputSchemaManifest totalAmount = new YamlSkillManifest.OutputSchemaManifest();
        totalAmount.setType("number");

        Map<String, YamlSkillManifest.OutputSchemaManifest> properties = new LinkedHashMap<>();
        properties.put("vendorName", vendorName);
        properties.put("totalAmount", totalAmount);
        root.setProperties(properties);
        root.setRequired(List.of("vendorName", "totalAmount"));
        root.setAdditionalProperties(false);
        return root;
    }

    private static YamlSkillManifest.OutputSchemaManifest schemaWithDateFormat() {
        YamlSkillManifest.OutputSchemaManifest root = new YamlSkillManifest.OutputSchemaManifest();
        root.setType("object");

        YamlSkillManifest.OutputSchemaManifest invoiceDate = new YamlSkillManifest.OutputSchemaManifest();
        invoiceDate.setType("string");
        invoiceDate.setFormat("date");

        root.setProperties(Map.of("invoiceDate", invoiceDate));
        root.setRequired(List.of("invoiceDate"));
        root.setAdditionalProperties(false);
        return root;
    }

    private static YamlSkillManifest.OutputSchemaManifest nullableSchema() {
        YamlSkillManifest.OutputSchemaManifest root = new YamlSkillManifest.OutputSchemaManifest();
        root.setType("object");

        YamlSkillManifest.OutputSchemaManifest driverName = new YamlSkillManifest.OutputSchemaManifest();
        driverName.setType("string");
        driverName.setNullable(true);

        root.setProperties(Map.of("driverName", driverName));
        root.setRequired(List.of("driverName"));
        root.setAdditionalProperties(false);
        return root;
    }

    private static YamlSkillManifest.OutputSchemaManifest wideSchema() {
        YamlSkillManifest.OutputSchemaManifest root = new YamlSkillManifest.OutputSchemaManifest();
        root.setType("object");

        Map<String, YamlSkillManifest.OutputSchemaManifest> properties = new LinkedHashMap<>();
        List<String> required = new ArrayList<>();
        for (String field : List.of("vendorName", "invoiceNumber", "invoiceDate", "totalAmount", "currency", "status")) {
            YamlSkillManifest.OutputSchemaManifest property = new YamlSkillManifest.OutputSchemaManifest();
            property.setType("string");
            properties.put(field, property);
            required.add(field);
        }
        root.setProperties(properties);
        root.setRequired(required);
        root.setAdditionalProperties(false);
        return root;
    }

    private static ChatClientRequest request(String text) {
        return new ChatClientRequest(new Prompt(text), Map.of());
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
}
