package com.lokiscale.loomspan.internal.chat;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.AdvisorTraceContext;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContractCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
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

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SkillAdvisorResolverTests {

    private final DefaultSkillAdvisorResolver resolver =
            new DefaultSkillAdvisorResolver(
                    new DefaultExecutionStateService(Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC)));

    @Test
    void returnsEmptyAdvisorListForSkillWithoutLinter() {
        assertThat(resolver.resolve(definition(false))).isEmpty();
    }

    @Test
    void createsLinterAdvisorForSkillWithRegexLinter() {
        assertThat(resolver.resolve(definition(true)))
                .singleElement()
                .isInstanceOf(LinterCallAdvisor.class);
    }

    @Test
    void createsOutputSchemaAdvisorBeforeLinterAdvisor() {
        assertThat(resolver.resolve(definition(true, true)))
                .hasSize(2)
                .element(0).isInstanceOf(OutputSchemaCallAdvisor.class);
        assertThat(resolver.resolve(definition(true, true)))
                .element(1).isInstanceOf(LinterCallAdvisor.class);
    }

    @Test
    void createsOutputSchemaThenEvidenceThenLinterAdvisorOrder() {
        assertThat(resolver.resolve(definition(true, true, true)))
                .hasSize(3)
                .element(0).isInstanceOf(OutputSchemaCallAdvisor.class);
        assertThat(resolver.resolve(definition(true, true, true)))
                .element(1).isInstanceOf(EvidenceContractCallAdvisor.class);
        assertThat(resolver.resolve(definition(true, true, true)))
                .element(2).isInstanceOf(LinterCallAdvisor.class);
    }

    @Test
    void rethrowsManagedSessionAdvisorMutationFailures() {
        ExecutionStateService failingStateService = new DefaultExecutionStateService(
                Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC)) {
            @Override
            public void recordAdvisorRequestMutation(com.lokiscale.loomspan.internal.core.LoomspanSession session,
                                                     AdvisorTraceContext context,
                                                     Object payload) {
                throw new IllegalStateException("boom");
            }
        };
        DefaultSkillAdvisorResolver failingResolver = new DefaultSkillAdvisorResolver(failingStateService);
        CallAdvisor advisor = (CallAdvisor) failingResolver.resolve(definition(true)).getFirst();
        LoomspanSessionRunner runner = new LoomspanSessionRunner(3);

        assertThatThrownBy(() -> runner.callWithNewSession("test.entry", session ->
                advisor.adviseCall(request("Write YAML"), new RecordingChain(List.of("bad", "OK")))))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("boom");
    }

    private YamlSkillDefinition definition(boolean withLinter) {
        return definition(withLinter, false);
    }

    private YamlSkillDefinition definition(boolean withLinter, boolean withOutputSchema) {
        return definition(withLinter, withOutputSchema, false);
    }

    private YamlSkillDefinition definition(boolean withLinter, boolean withOutputSchema, boolean withEvidenceContract) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName(withLinter ? "lintedSkill" : "plain.skill");
        manifest.setDescription(manifest.getName());
        manifest.setModel("gpt-5");
        if (withOutputSchema) {
            YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
            schema.setType("object");
            YamlSkillManifest.OutputSchemaManifest vendorName = new YamlSkillManifest.OutputSchemaManifest();
            vendorName.setType("string");
            if (withEvidenceContract) {
                vendorName.setEvidence("invoiceParser");
            }
            schema.setProperties(java.util.Map.of("vendorName", vendorName));
            schema.setRequired(java.util.List.of("vendorName"));
            schema.setAdditionalProperties(false);
            manifest.setOutputSchema(schema);
            manifest.setOutputSchemaMaxRetries(2);
        }
        if (withLinter) {
            YamlSkillManifest.RegexManifest regex = new YamlSkillManifest.RegexManifest();
            regex.setPattern("^OK.*$");
            regex.setMessage("must start with OK");
            YamlSkillManifest.LinterManifest linter = new YamlSkillManifest.LinterManifest();
            linter.setType("regex");
            linter.setMaxRetries(2);
            linter.setRegex(regex);
            manifest.setLinter(linter);
        }
        return new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                new EffectiveSkillExecutionConfiguration("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium"),
                withEvidenceContract
                        ? com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(
                                java.util.Map.of("vendorName", "invoiceParser"))
                        : EvidenceContract.empty());
    }

    private static ChatClientRequest request(String text) {
        return new ChatClientRequest(new Prompt(text), Map.of());
    }

    private static final class RecordingChain implements CallAdvisorChain {

        private final List<String> responses;
        private final List<ChatClientRequest> requests;
        private final AtomicInteger index;
        private final AtomicBoolean consumed;

        private RecordingChain(List<String> responses) {
            this(responses, new ArrayList<>(), new AtomicInteger(), new AtomicBoolean());
        }

        private RecordingChain(List<String> responses,
                               List<ChatClientRequest> requests,
                               AtomicInteger index,
                               AtomicBoolean consumed) {
            this.responses = responses;
            this.requests = requests;
            this.index = index;
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
            return new RecordingChain(responses, requests, index, new AtomicBoolean());
        }
    }
}
