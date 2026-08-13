package com.lokiscale.loomspan.internal.springai;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.linter.LinterCallAdvisor;
import com.lokiscale.loomspan.internal.core.*;
import com.lokiscale.loomspan.internal.model.ModelInteractionMode;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.runtime.attachment.RenderedMissionInput;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import com.lokiscale.loomspan.internal.runtime.usage.*;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.core.TestLoomspanSessions;
import io.micrometer.observation.ObservationRegistry;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import org.junit.jupiter.api.Test;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.DefaultResourceLoader;

import java.time.Clock;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import java.util.regex.Pattern;

import static org.assertj.core.api.Assertions.assertThat;

class SpringAiChatClientAssemblerIntegrationTest
{
    @Test
    void productionStandardAssemblyHasOneToolLoopInsideSemanticRetry() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(toolCall("call-1", "first"));
            server.enqueue(text("invalid"));
            server.enqueue(toolCall("call-2", "second"));
            server.enqueue(text("OK: corrected"));
            Harness harness = harness(server, true);

            String result = harness.call(ModelInteractionMode.STANDARD);

            assertThat(result).isEqualTo("OK: corrected");
            assertThat(server.getRequestCount()).isEqualTo(4);
            assertThat(harness.toolExecutions).hasValue(2);
            assertThat(harness.session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(4);
        }
    }

    @Test
    void productionStepAssemblyKeepsOneToolLoopAndFiltersSemanticValidators() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(toolCall("call-1", "step"));
            server.enqueue(text("invalid"));
            Harness harness = harness(server, true);

            String result = harness.call(ModelInteractionMode.STEP_EXECUTION);

            assertThat(result).isEqualTo("invalid");
            assertThat(server.getRequestCount()).isEqualTo(2);
            assertThat(harness.toolExecutions).hasValue(1);
            assertThat(harness.session.getSessionUsage().orElseThrow().providerAttempts()).isEqualTo(2);
        }
    }

    @Test
    void productionAssemblyWithoutCapabilitiesDoesNotCreateToolTurns() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(text("OK: direct"));
            Harness harness = harness(server, false);

            assertThat(harness.call(ModelInteractionMode.STANDARD)).isEqualTo("OK: direct");
            assertThat(server.getRequestCount()).isOne();
            assertThat(harness.toolExecutions).hasValue(0);
            assertThat(harness.session.getSessionUsage().orElseThrow().providerAttempts()).isOne();
        }
    }

    @Test
    void productionToolAdapterPreservesExplicitNullArguments() throws Exception
    {
        try (MockWebServer server = new MockWebServer())
        {
            server.enqueue(toolCallArguments("call-null", "{\\\"value\\\":null}"));
            server.enqueue(text("OK: null accepted"));
            Harness harness = harness(server, true);

            assertThat(harness.call(ModelInteractionMode.STANDARD)).isEqualTo("OK: null accepted");
            assertThat(harness.observedArguments.get()).containsEntry("value", null);
        }
    }

    private static Harness harness(MockWebServer server, boolean includeTool)
    {
        LoomspanProperties.ConnectionProperties properties = new LoomspanProperties.ConnectionProperties();
        properties.setDriver(AiDriver.OPENAI);
        properties.setApiKey("assembler-key");
        properties.setBaseUrl(server.url("/v1").toString());
        properties.getProviderRetry().setEnabled(false);
        ProviderConnectionRuntime runtime = new SpringAiProviderIntegration(new DefaultResourceLoader()).create("openai", properties);
        DefaultSessionUsageService usage = new DefaultSessionUsageService(
                new LoomspanProperties().getSession().getQuotas(), new NoOpUsageMetricsRecorder());
        DefaultExecutionStateService state = new DefaultExecutionStateService(Clock.systemUTC(), usage);
        var resolver = (com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver) ignored -> List.of(
                new LinterCallAdvisor("test.skill", "regex", Pattern.compile("^OK:.*$"), "Return OK", 1, fact -> { }));
        SpringAiModelInteractionFactory factory = new SpringAiModelInteractionFactory(
                (skill, configuration) -> runtime, resolver, state, new ModelUsageExtractor(), usage,
                new SpringAiChatOptionsContributor(), ObservationRegistry.NOOP);
        LoomspanSession session = TestLoomspanSessions.withId("assembler", "test.skill", 8);
        state.openMissionFrame(session, "test.skill", Map.of());
        state.openFrame(session, TraceFrameType.MODEL_CALL, "test.skill#model", Map.of());
        AtomicInteger executions = new AtomicInteger();
        AtomicReference<Map<String, Object>> observedArguments = new AtomicReference<>();
        BoundCapability capability = capability(executions, observedArguments);
        return new Harness(factory, definition(), session, includeTool ? List.of(capability) : List.of(), executions,
                observedArguments);
    }

    private record Harness(SpringAiModelInteractionFactory factory, YamlSkillDefinition definition,
            LoomspanSession session, List<BoundCapability> capabilities, AtomicInteger toolExecutions,
            AtomicReference<Map<String, Object>> observedArguments)
    {
        String call(ModelInteractionMode mode)
        {
            return com.lokiscale.loomspan.internal.core.SessionContextRunner.callWithSession(session, () ->
                    factory.create(definition, mode).call(new ModelInteractionRequest(
                            "system", new RenderedMissionInput("user", List.of(), Map.of()),
                            new ModelTraceContext(ModelExecutionIdentity.from(definition.requireExecutionConfiguration()),
                                    "test.skill", "mission"), capabilities, false)).content());
        }
    }

    private static YamlSkillDefinition definition()
    {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("test.skill");
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest,
                new EffectiveSkillExecutionConfiguration("gpt-test", "openai", AiDriver.OPENAI, "gpt-test", null));
    }

    private static BoundCapability capability(AtomicInteger executions,
            AtomicReference<Map<String, Object>> observedArguments)
    {
        CapabilityMetadata metadata = new CapabilityMetadata("test:lookup", "lookup", "Look up",
                SkillExecutionDescriptor.none(), Set.of(), ignored -> null, CapabilityKind.YAML_SKILL,
                new CapabilityToolDescriptor("lookup", "Look up",
                        "{\"type\":\"object\",\"properties\":{\"value\":{\"type\":\"string\"}}}"),
                SkillInputContract.genericObject(), null);
        return new BoundCapability(metadata, (arguments, task) -> {
            executions.incrementAndGet();
            observedArguments.set(arguments);
            return "looked-up-" + arguments.get("value");
        });
    }

    private static MockResponse toolCall(String id, String value)
    {
        return toolCallArguments(id, "{\\\"value\\\":\\\"%s\\\"}".formatted(value));
    }

    private static MockResponse toolCallArguments(String id, String arguments)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"tool","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":null,
                  "tool_calls":[{"id":"%s","type":"function","function":{"name":"lookup","arguments":"%s"}}]},"finish_reason":"tool_calls"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(id, arguments));
    }

    private static MockResponse text(String value)
    {
        return new MockResponse().setHeader("Content-Type", "application/json").setBody("""
                {"id":"text","object":"chat.completion","created":1,"model":"gpt-test",
                 "choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],
                 "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}
                """.formatted(value));
    }
}
