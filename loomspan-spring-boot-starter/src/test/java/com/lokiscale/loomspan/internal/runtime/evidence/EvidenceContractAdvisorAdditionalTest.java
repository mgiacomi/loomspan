package com.lokiscale.loomspan.internal.runtime.evidence;

import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
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
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class EvidenceContractAdvisorAdditionalTest
{
    @Test
    void unsupportedRequiredClaimFailsAfterToolFreeRetriesAndPreservesExpressionDiagnostics()
    {
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setProperties(Map.of("result", scalar("string")));
        schema.setRequired(List.of("result"));
        List<EvidenceCoverageResult> failures = new ArrayList<>();
        EvidenceContractCallAdvisor advisor = new EvidenceContractCallAdvisor(
                "handleIncident",
                TestEvidenceContracts.compiled(Map.of(
                        "result", "classifyIncident and (investigateNetwork or investigateApp)")),
                new EvidenceBackedOutputValidator(),
                1,
                ignored -> { },
                failures::add,
                ignored -> { });
        RecordingChain chain = new RecordingChain(List.of(
                "{\"result\":\"network issue\"}",
                "{\"result\":\"network issue\"}"));

        new LoomspanSessionRunner(3).callWithNewSession("test.entry", session ->
        {
            session.addSuccessfulDirectSkill("classifyIncident");
            assertThatThrownBy(() -> advisor.adviseCall(request(), chain))
                    .isInstanceOf(LoomspanEvidenceValidationException.class)
                    .hasMessageContaining("handleIncident")
                    .hasMessageContaining("2 attempt");
            return null;
        });

        assertThat(chain.requests).hasSize(2);
        assertThat(chain.copyInvocations.get()).isEqualTo(2);
        assertThat(chain.requests.get(1).prompt().getSystemMessage().getText())
                .contains("Do NOT call any tools again")
                .contains("classifyIncident and (investigateNetwork or investigateApp)")
                .contains("successfully completed direct skills: [classifyIncident]");
        assertThat(failures).hasSize(2).allSatisfy(result ->
        {
            assertThat(result.satisfiedSkills()).containsExactly("classifyIncident");
            assertThat(result.issues()).singleElement().satisfies(issue ->
            {
                assertThat(issue.requiredExpression())
                        .isEqualTo("classifyIncident and (investigateNetwork or investigateApp)");
                assertThat(issue.unsatisfiedRequirements().getFirst().children().getFirst().mode()).isEqualTo("any");
            });
        });
    }

    private static YamlSkillManifest.OutputSchemaManifest scalar(String type)
    {
        YamlSkillManifest.OutputSchemaManifest scalar = new YamlSkillManifest.OutputSchemaManifest();
        scalar.setType(type);
        return scalar;
    }

    private static ChatClientRequest request()
    {
        return new ChatClientRequest(new Prompt(List.of(
                new SystemMessage("Return the required JSON result."),
                new UserMessage("Handle incident"))), Map.of());
    }

    private static final class RecordingChain implements CallAdvisorChain
    {
        private final List<String> responses;
        private final List<ChatClientRequest> requests;
        private final AtomicInteger index;
        private final AtomicInteger copyInvocations;
        private final AtomicBoolean consumed;

        private RecordingChain(List<String> responses)
        {
            this(responses, new ArrayList<>(), new AtomicInteger(), new AtomicInteger(), new AtomicBoolean());
        }

        private RecordingChain(List<String> responses,
                List<ChatClientRequest> requests,
                AtomicInteger index,
                AtomicInteger copyInvocations,
                AtomicBoolean consumed)
        {
            this.responses = responses;
            this.requests = requests;
            this.index = index;
            this.copyInvocations = copyInvocations;
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
            copyInvocations.incrementAndGet();
            return new RecordingChain(responses, requests, index, copyInvocations, new AtomicBoolean());
        }
    }
}
