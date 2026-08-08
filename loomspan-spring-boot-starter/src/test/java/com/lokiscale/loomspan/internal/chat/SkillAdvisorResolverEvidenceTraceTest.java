package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContractCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.chat.prompt.Prompt;
import org.springframework.core.io.ByteArrayResource;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SkillAdvisorResolverEvidenceTraceTest
{
    private static final Clock CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void directEvidenceAdvisorFailureRecordsStructuredExpressionDiagnostics()
    {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK);
        CallAdvisor advisor = evidenceAdvisor(new DefaultSkillAdvisorResolver(stateService));
        List<TraceRecord> records = new ArrayList<>();
        RecordingChain chain = new RecordingChain(List.of(
                "{\"vendorName\":\"Acme\"}",
                "{\"vendorName\":\"Acme\"}",
                "{\"vendorName\":\"Acme\"}"));

        assertThatThrownBy(() -> new LoomspanSessionRunner(3).callWithNewSession("test.entry", session ->
        {
            try
            {
                return advisor.adviseCall(request(), chain);
            }
            finally
            {
                session.readTraceRecords(records::add);
            }
        }))
                .isInstanceOf(RuntimeException.class)
                .hasMessageContaining("3 attempt(s)");

        assertThat(chain.requests).hasSize(3);
        assertThat(records)
                .filteredOn(record -> record.recordType() == TraceRecordType.EVIDENCE_VALIDATION_FAILED)
                .hasSize(3)
                .allSatisfy(record ->
                {
                    assertThat(record.metadata())
                            .containsEntry("unsatisfiedClaims", List.of("vendorName"))
                            .containsEntry("requiredExpressions", Map.of("vendorName", "invoiceParser"));
                    assertThat(record.metadata().get("satisfiedSkills")).isEqualTo(List.of());
                    assertThat(record.data().path("requiredExpressions").path("vendorName").asText())
                            .isEqualTo("invoiceParser");
                    assertThat(record.data().path("issues").get(0).path("requiredExpression").asText())
                            .isEqualTo("invoiceParser");
                    assertThat(record.data().path("issues").get(0).path("unsatisfiedRequirements"))
                            .hasSize(1);
                    assertThat(record.metadata()).doesNotContainKey("missingEvidence");
                    assertThat(record.data().has("missingEvidence")).isFalse();
                });
    }

    @Test
    void directEvidenceAdvisorPassRecordsSuccessfulSkillTruthSet()
    {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(CLOCK);
        CallAdvisor advisor = evidenceAdvisor(new DefaultSkillAdvisorResolver(stateService));
        List<TraceRecord> records = new ArrayList<>();

        new LoomspanSessionRunner(3).callWithNewSession("test.entry", session ->
        {
            session.addSuccessfulDirectSkill("invoiceParser");
            ChatClientResponse response = advisor.adviseCall(
                    request(),
                    new RecordingChain(List.of("{\"vendorName\":\"Acme\"}")));
            session.readTraceRecords(records::add);
            return response;
        });

        assertThat(records)
                .filteredOn(record -> record.recordType() == TraceRecordType.EVIDENCE_VALIDATION_PASSED)
                .singleElement()
                .satisfies(record ->
                {
                    assertThat(record.metadata().get("claims"))
                            .isEqualTo(List.of("vendorName"));
                    assertThat(record.metadata().get("satisfiedSkills"))
                            .isEqualTo(List.of("invoiceParser"));
                    assertThat(record.data().path("requiredExpressions").path("vendorName").asText())
                            .isEqualTo("invoiceParser");
                    assertThat(record.data().path("issues").isEmpty()).isTrue();
                    assertThat(record.data().has("missingEvidence")).isFalse();
                });
    }

    private static CallAdvisor evidenceAdvisor(DefaultSkillAdvisorResolver resolver)
    {
        return (CallAdvisor) resolver.resolve(definition()).stream()
                .filter(EvidenceContractCallAdvisor.class::isInstance)
                .findFirst()
                .orElseThrow();
    }

    private static YamlSkillDefinition definition()
    {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("vendorSkill");
        manifest.setDescription("Returns a vendor");
        manifest.setModel("gpt-5");
        manifest.setOutputSchemaMaxRetries(2);

        YamlSkillManifest.OutputSchemaManifest vendorName = new YamlSkillManifest.OutputSchemaManifest();
        vendorName.setType("string");
        vendorName.setEvidence("invoiceParser");
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setProperties(Map.of("vendorName", vendorName));
        schema.setRequired(List.of("vendorName"));
        schema.setAdditionalProperties(false);
        manifest.setOutputSchema(schema);

        return new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                new EffectiveSkillExecutionConfiguration(
                        "gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium"),
                com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(
                        Map.of("vendorName", "invoiceParser")));
    }

    private static ChatClientRequest request()
    {
        return new ChatClientRequest(new Prompt("Return vendor JSON"), Map.of());
    }

    private static final class RecordingChain implements CallAdvisorChain
    {
        private final List<String> responses;
        private final List<ChatClientRequest> requests;
        private final AtomicInteger index;
        private final AtomicBoolean consumed;

        private RecordingChain(List<String> responses)
        {
            this(responses, new ArrayList<>(), new AtomicInteger(), new AtomicBoolean());
        }

        private RecordingChain(List<String> responses,
                List<ChatClientRequest> requests,
                AtomicInteger index,
                AtomicBoolean consumed)
        {
            this.responses = responses;
            this.requests = requests;
            this.index = index;
            this.consumed = consumed;
        }

        @Override
        public ChatClientResponse nextCall(ChatClientRequest request)
        {
            if (!consumed.compareAndSet(false, true))
            {
                throw new IllegalStateException("No CallAdvisors available to execute");
            }
            requests.add(request.copy());
            String response = responses.get(Math.min(index.getAndIncrement(), responses.size() - 1));
            return ChatClientResponse.builder()
                    .chatResponse(new ChatResponse(List.of(new Generation(new AssistantMessage(response)))))
                    .build();
        }

        @Override
        public List<CallAdvisor> getCallAdvisors()
        {
            return List.of();
        }

        @Override
        public CallAdvisorChain copy(CallAdvisor after)
        {
            return new RecordingChain(responses, requests, index, new AtomicBoolean());
        }
    }
}
