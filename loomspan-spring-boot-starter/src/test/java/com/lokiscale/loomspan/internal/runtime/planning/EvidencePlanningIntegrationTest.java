package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.SimpleChatClient;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.definition.ToolDefinition;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class EvidencePlanningIntegrationTest
{
    private static final Clock FIXED_CLOCK = Clock.fixed(
            Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);

    @Test
    void acceptsEitherInvestigatorWithoutRequiringBothAndRendersTheCanonicalExpression()
    {
        for (String investigator : List.of("investigateNetwork", "investigateApp"))
        {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-" + investigator, "test.entry", 3);
            SimpleChatClient client = new SimpleChatClient(incidentPlan(investigator), "unused");

            ExecutionPlan result = planningService.initializePlan(
                    session,
                    "handle incident",
                    null,
                    incidentDefinition(),
                    client,
                    incidentTools()).orElseThrow();

            assertThat(result.tasks()).extracting(PlanTask::capabilityName)
                    .contains("classifyIncident", investigator, "draftIncidentResponse")
                    .doesNotContain(investigator.equals("investigateNetwork") ? "investigateApp" : "investigateNetwork");
            assertThat(client.getSystemMessagesSeen().getFirst())
                    .contains("classifyIncident and (investigateNetwork or investigateApp)")
                    .contains("For an 'or' group, include any one alternative")
                    .doesNotContain("[investigateNetwork, investigateApp] tool(s)");
        }
    }

    @Test
    void failedPlanningTraceRetainsStructuredBooleanRequirementsWithoutLegacyAliases()
    {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-gap", "test.entry", 3);
        SimpleChatClient client = new SimpleChatClient(classificationOnlyPlan(), "unused");

        assertThatThrownBy(() -> planningService.initializePlan(
                session,
                "handle incident",
                null,
                incidentDefinition(),
                client,
                incidentTools()))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Evidence coverage validation failed");

        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.PLAN_VALIDATION_FAILED
                        && java.util.List.of("evidence-coverage").equals(record.metadata().get("issueCodes")))
                .isNotEmpty()
                .allSatisfy(record ->
                {
                    assertThat(record.metadata()).containsKeys(
                            "unsatisfiedClaims", "requiredExpressions", "satisfiedSkills", "unsatisfiedRequirements");
                    assertThat(record.metadata()).doesNotContainKey("missingEvidence");
                    assertThat(record.data().toString())
                            .contains("classifyIncident and (investigateNetwork or investigateApp)")
                            .contains("\"mode\":\"any\"");
                });
        assertThat(client.getSystemMessagesSeen()).anySatisfy(message -> assertThat(message)
                .contains("classifyIncident and (investigateNetwork or investigateApp)")
                .contains("already planned: [classifyIncident, draftIncidentResponse]"));
    }

    private static YamlSkillDefinition incidentDefinition()
    {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("handleIncident");
        manifest.setDescription("Handle an incident");
        manifest.setModel("gpt-5");
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setAdditionalProperties(false);
        schema.setProperties(Map.of(
                "severity", scalar("string"),
                "likelyCause", scalar("string"),
                "userMessage", scalar("string")));
        schema.setRequired(List.of("severity", "userMessage"));
        manifest.setOutputSchema(schema);
        Map<String, String> evidence = Map.of(
                "severity", "classifyIncident",
                "likelyCause", "classifyIncident and (investigateNetwork or investigateApp)",
                "userMessage", "draftIncidentResponse");
        EffectiveSkillExecutionConfiguration configuration = new EffectiveSkillExecutionConfiguration(
                "gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium");
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                configuration,
                com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(evidence));
    }

    private static YamlSkillManifest.OutputSchemaManifest scalar(String type)
    {
        YamlSkillManifest.OutputSchemaManifest scalar = new YamlSkillManifest.OutputSchemaManifest();
        scalar.setType(type);
        return scalar;
    }

    private static List<ToolCallback> incidentTools()
    {
        return List.of(
                tool("classifyIncident"),
                tool("investigateNetwork"),
                tool("investigateApp"),
                tool("draftIncidentResponse"));
    }

    private static ToolCallback tool(String name)
    {
        ToolCallback callback = mock(ToolCallback.class);
        when(callback.getToolDefinition()).thenReturn(
                ToolDefinition.builder().name(name).description("Use " + name).inputSchema("{}").build());
        return callback;
    }

    private static ExecutionPlan incidentPlan(String investigator)
    {
        return plan(List.of("classifyIncident", investigator, "draftIncidentResponse"));
    }

    private static ExecutionPlan classificationOnlyPlan()
    {
        return plan(List.of("classifyIncident", "draftIncidentResponse", "draftIncidentResponse"));
    }

    private static ExecutionPlan plan(List<String> capabilities)
    {
        List<PlanTask> tasks = java.util.stream.IntStream.range(0, capabilities.size())
                .mapToObj(index -> new PlanTask(
                        "task-" + index,
                        "Task " + index,
                        PlanTaskStatus.PENDING,
                        capabilities.get(index),
                        "Use " + capabilities.get(index),
                        index == 0 ? List.of() : List.of("task-" + (index - 1)),
                        List.of("result"),
                        false,
                        null))
                .toList();
        return new ExecutionPlan(
                "plan-incident",
                "handleIncident",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                tasks);
    }
}
