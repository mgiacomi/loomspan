package com.lokiscale.loomspan.internal.runtime.evidence;

import com.fasterxml.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClientRequest;
import org.springframework.ai.chat.client.ChatClientResponse;
import org.springframework.ai.chat.client.advisor.api.CallAdvisor;
import org.springframework.ai.chat.client.advisor.api.CallAdvisorChain;
import org.springframework.ai.chat.messages.AssistantMessage;
import org.springframework.ai.chat.messages.SystemMessage;
import org.springframework.ai.chat.messages.UserMessage;
import org.springframework.ai.chat.model.ChatResponse;
import org.springframework.ai.chat.model.Generation;
import org.springframework.ai.chat.prompt.Prompt;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;

class EvidenceContractTests {

    @Test
    void preservesDiagnosticClaimExpressionAndSkillOrder() {
        LinkedHashMap<String, String> requiredExpressions = new LinkedHashMap<>();
        requiredExpressions.put("vendorName", "invoiceParser");
        requiredExpressions.put("isDuplicate", "invoiceParser and expenseLookup");
        EvidenceCoverageResult result = new EvidenceCoverageResult(
                new LinkedHashSet<>(List.of("vendorName", "isDuplicate")),
                requiredExpressions,
                new LinkedHashSet<>(List.of("invoiceParser", "expenseLookup")),
                List.of());

        assertThat(result.evaluatedClaims()).containsExactly("vendorName", "isDuplicate");
        assertThat(result.requiredExpressions().keySet()).containsExactly("vendorName", "isDuplicate");
        assertThat(result.satisfiedSkills()).containsExactly("invoiceParser", "expenseLookup");
    }

    @Test
    void normalizesClaimsToolsAndPresentClaimLookups() throws Exception {
        EvidenceContract contract = TestEvidenceContracts.compiled(Map.of(
                "vendorName", "invoiceParser",
                "isDuplicate", "invoiceParser and expenseLookup"));
        EvidenceBackedOutputValidator validator = new EvidenceBackedOutputValidator();

        assertThat(contract.canonicalExpressionForClaim("VENDORNAME")).isEqualTo("invoiceParser");
        assertThat(contract.canonicalExpressionForClaim("isDuplicate")).isEqualTo("invoiceParser and expenseLookup");

        EvidenceCoverageResult result = validator.validate(
                JsonMapper.builder().findAndAddModules().build().readTree("""
                        {"vendorName":"Acme","isDuplicate":false}
                        """),
                contract,
                Set.of("invoiceParser"));
        assertThat(result.complete()).isFalse();
        assertThat(result.issues()).singleElement()
                .extracting(EvidenceCoverageIssue::claimName)
                .isEqualTo("isDuplicate");
    }

    @Test
    void evidenceAdvisorPreservesExistingSkillPromptOnRetry() {
        EvidenceContractCallAdvisor advisor = new EvidenceContractCallAdvisor(
                "evidence.skill",
                TestEvidenceContracts.compiled(Map.of("vendorName", "invoiceParser")),
                new EvidenceBackedOutputValidator(),
                1,
                result -> {
                },
                result -> {
                },
                fact -> {
                });
        RecordingChain chain = new RecordingChain(List.of(
                "{\"vendorName\":\"Acme\"}",
                "{}"));

        new LoomspanSessionRunner(3).callWithNewSession("test.entry", session -> {
            ChatClientResponse response = advisor.adviseCall(
                    request("Return JSON", "SKILL_PROMPT_SENTINEL\n\nBase instructions."),
                    chain);

            assertThat(response.chatResponse().getResult().getOutput().getText()).isEqualTo("{}");
            return null;
        });

        assertThat(chain.requests).hasSize(2);
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .startsWith("SKILL_PROMPT_SENTINEL")
                .contains("Evidence validation failed")
                .contains("Use only results already gathered");
    }

    private static ChatClientRequest request(String userText, String systemText) {
        return new ChatClientRequest(new Prompt(List.of(
                new SystemMessage(systemText),
                new UserMessage(userText))), Map.of());
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
    }
}
