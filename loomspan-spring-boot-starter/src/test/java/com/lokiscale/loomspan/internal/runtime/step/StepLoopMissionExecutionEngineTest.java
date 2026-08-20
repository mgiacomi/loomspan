package com.lokiscale.loomspan.internal.runtime.step;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus;
import com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.DefaultCapabilityInvoker;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.content.Media;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.definition.ToolDefinition;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.Resource;
import org.springframework.lang.Nullable;
import org.springframework.util.MimeType;

import java.net.URL;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class StepLoopMissionExecutionEngineTest {

    private static final String BLOCK_UNTIL_INTERRUPTED = "__block_until_interrupted__";
    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);
    private static final EffectiveSkillExecutionConfiguration EXECUTION_CONFIGURATION =
            new EffectiveSkillExecutionConfiguration("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium");

    @Test
    void recordsStepModelFailureBeforeItsModelFrameCloses() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        PlanningService planningService = new InitializingPlanningService(stateService, singleTaskPlan());
        SequenceChatClient chatClient = new SequenceChatClient();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "step-model-failure", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);
            assertThatThrownBy(() -> executeMission(engine, session, definition(), chatClient, List.of()))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("No more queued chat responses");
        }

        List<TraceRecord> records = readRecords(session);
        TraceRecord error = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                .findFirst()
                .orElseThrow();
        TraceRecord modelClose = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                .filter(record -> record.frameType() == TraceFrameType.MODEL_CALL)
                .findFirst()
                .orElseThrow();
        assertThat(error.frameType()).isEqualTo(TraceFrameType.MODEL_CALL);
        assertThat(error.frameId()).isEqualTo(modelClose.frameId());
        assertThat(error.sequence()).isLessThan(modelClose.sequence());
        assertThat(error.data().path("diagnostics")).hasSize(1);
    }

    @Test
    void executesNewlyUnblockedTasksAcrossMultipleSteps() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = twoTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-2","toolName":"expenseLookup","toolArguments":{"invoiceId":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Mission complete"}
                """);

        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-1", "test.entry", 3);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"), tool("expenseLookup", "{\"matches\":[]}")));

            assertThat(response).isEqualTo("Mission complete");
        }

        ExecutionPlan finalPlan = stateService.currentPlan(session).orElseThrow();
        assertThat(finalPlan.tasks()).extracting(PlanTask::status)
                .containsExactly(PlanTaskStatus.COMPLETED, PlanTaskStatus.COMPLETED);
        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("READY TASKS", "t-2");
        assertThat(chatClient.systemMessagesSeen().get(1)).doesNotContain("t-2: Look up expenses (waiting on: t-1)");
        List<TraceRecord> records = readRecords(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.STEP_STARTED)).hasSize(3);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.STEP_COMPLETED)).hasSize(3);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.STEP_FAILED);
    }

    @Test
    void retriesInvalidActionBeforeProceeding() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Too early"}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-retry", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            YamlSkillDefinition definition = definitionWithPrompt();
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor, definition);

            String response = executeMission(
                    engine,
                    session,
                    definition,
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records).anyMatch(record -> record.recordType() == TraceRecordType.STEP_ACTION_REJECTED);
        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("YOUR PREVIOUS ACTION WAS INVALID")
                .containsOnlyOnce("STEP_PROMPT_SENTINEL");
    }

    @Test
    void stepLoopIncludesSkillPromptInStepAndFinalResponsePrompts() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-prompt", "test.entry", 3);
        YamlSkillDefinition definition = definitionWithPrompt();

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor, definition);

            String response = executeMission(
                    engine,
                    session,
                    definition,
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen()).allSatisfy(prompt -> assertThat(prompt)
                .startsWith("STEP_PROMPT_SENTINEL")
                .contains("You are executing a planned mission step by step."));

    }

    @Test
    void retriesMissingActionFieldBeforeProceeding() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-missing-action", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records).anyMatch(record -> record.recordType() == TraceRecordType.STEP_ACTION_REJECTED
                && String.valueOf(record.metadata().get("reason")).contains("Step action type"));
        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
    }

    @Test
    void surfacesToolFailureAsExplicitTerminalFailure() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-failure", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(failingTool("invoiceParser"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("tool 'invoiceParser' failed")
                    .hasMessageNotContaining("deadlock");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.STEP_FAILED))
                .singleElement()
                .satisfies(record -> assertThat(record.metadata().get("failureId")).isNotNull());
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.STEP_COMPLETED);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.ERROR_RECORDED
                && String.valueOf(record.data()).contains("deadlock"));
    }

    @Test
    void surfacesModelFailureAsExplicitTerminalFailure() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        PlanningService planningService = new InitializingPlanningService(stateService, singleTaskPlan());
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-model-failure", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);
            assertThatThrownBy(() -> executeMission(engine, session, definition(), new SequenceChatClient(), List.of()))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("No more queued chat responses");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.STEP_FAILED)).hasSize(1);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.STEP_COMPLETED);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                .filter(record -> record.frameType() == TraceFrameType.STEP_EXECUTION))
                .allSatisfy(record -> assertThat(record.metadata()).containsEntry("status", "failed"));
    }

    @Test
    void retriesFinalResponseUntilOutputSchemaPasses() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"plain text"}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"{\\\"result\\\":\\\"Finished\\\"}"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-schema", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    definitionWithOutputSchema());

            String response = executeMission(
                    engine,
                    session,
                    definitionWithOutputSchema(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("{\"result\":\"Finished\"}");
        }

        assertThat(session.getLastOutputSchemaOutcome()).isPresent();
        assertThat(session.getLastOutputSchemaOutcome().orElseThrow().status()).isEqualTo(OutputSchemaOutcomeStatus.PASSED);
        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("Final response violates output_schema");
    }

    @Test
    void evidenceRetriesDoNotConsumeOutputSchemaRetryBudget() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":{"result":"Finished","reasoning":"unsupported"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":{"result":"Finished"}}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-evidence", "test.entry", 3);
        session.addSuccessfulDirectSkill("invoiceParser");

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    definitionWithOutputSchemaAndEvidenceContract());

            String response = executeMission(
                    engine,
                    session,
                    definitionWithOutputSchemaAndEvidenceContract(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("{\"result\":\"Finished\"}");
        }

        assertThat(session.getLastOutputSchemaOutcome()).isPresent();
        assertThat(session.getLastOutputSchemaOutcome().orElseThrow().attempt()).isEqualTo(1);
        assertThat(session.getLastOutputSchemaOutcome().orElseThrow().status()).isEqualTo(OutputSchemaOutcomeStatus.PASSED);
        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("unsupported by gathered evidence")
                .contains("requires successful completion of 'expenseLookup'")
                .contains("successfully completed direct skills: [invoiceParser]");
        List<TraceRecord> evidenceRecords = new ArrayList<>();
        session.readTraceRecords(evidenceRecords::add);
        assertThat(evidenceRecords).filteredOn(record -> record.recordType() == TraceRecordType.EVIDENCE_VALIDATION_FAILED)
                .singleElement()
                .satisfies(record ->
                {
                    assertThat(record.data().path("requiredExpressions").path("reasoning").asText()).isEqualTo("expenseLookup");
                    assertThat(record.data().path("satisfiedSkills")).hasSize(1);
                    assertThat(record.data().path("issues").get(0).path("unsatisfiedRequirements")).hasSize(1);
                    assertThat(record.data().has("missingEvidence")).isFalse();
                });
    }

    @Test
    void finalOnlyModeWrapsBarePayloadAsFinalResponseEnvelope() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {
                  "result": "Finished"
                }
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-bare-final-payload", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    definitionWithOutputSchema());

            String response = executeMission(
                    engine,
                    session,
                    definitionWithOutputSchema(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("{\"result\":\"Finished\"}");
        }

        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.STEP_ACTION_VALIDATED
                && "FINAL_RESPONSE".equals(String.valueOf(record.metadata().get("stepAction"))));
    }

    @Test
    void completedPlanRepairsInvalidCallToolByConstrainingPromptToFinalResponse() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-complete-plan-repair", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(0)).contains("All required plan tasks are already COMPLETE");
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("All required plan tasks are already completed. Return FINAL_RESPONSE instead of CALL_TOOL.")
                .contains("You must return a FINAL_RESPONSE action");
        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.STEP_ACTION_REJECTED
                && String.valueOf(record.metadata().get("reason")).contains("Return FINAL_RESPONSE instead of CALL_TOOL"));
    }

    @Test
    void retriesWhenConcreteToolSchemaRequiresMissingArguments() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-schema-aware-tool-args", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(toolWithSchema("invoiceParser", """
                            {
                              "type": "object",
                              "properties": {
                                "rawText": { "type": "string" }
                              },
                              "required": ["rawText"],
                              "additionalProperties": false
                            }
                            """, "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("missing_required")
                .contains("rawText")
                .contains("YOUR PREVIOUS ACTION WAS INVALID");
    }

    @Test
    void retriesWhenToolArgumentsContainPlaceholderSentinelValues() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"payload":"<canonical mission input>"}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"payload":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-placeholder-tool-args", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(toolWithSchema("invoiceParser", """
                            {
                              "type": "object",
                              "properties": {
                                "payload": { "type": "string" }
                              },
                              "required": ["payload"],
                              "additionalProperties": false
                            }
                            """, "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("unresolved placeholder values")
                .contains("payload")
                .contains("YOUR PREVIOUS ACTION WAS INVALID");
    }

    @Test
    void retriesToolArgumentValidationWithVerboseGuidanceAfterFirstFailure() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-verbose-retry", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(toolWithContract("invoiceParser", """
                            {
                              "type": "object",
                              "properties": {},
                              "additionalProperties": true
                            }
                            """, """
                            {
                              "type": "object",
                              "properties": {
                                "rawText": { "type": "string" }
                              },
                              "required": ["rawText"],
                              "additionalProperties": false
                            }
                            """, "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(0)).doesNotContain("Required fields:");
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("YOUR PREVIOUS ACTION WAS INVALID")
                .contains("Required fields: rawText")
                .contains("`rawText` must be a string");
    }

    @Test
    void usesCanonicalMissionInputForPlanningAndStepUserMessages() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-7"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-mission-input", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    "Execute YAML skill 'rootVisibleSkill' using the provided mission input object.",
                    Map.of("invoiceId", "INV-7"),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.userMessagesSeen()).isNotEmpty();
        assertThat(chatClient.userMessagesSeen().getFirst())
                .contains("Canonical mission input")
                .contains("\"invoiceId\" : \"INV-7\"");
    }

    @Test
    void stepLoopSendsAttachmentMediaOnExecutionSteps() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-7"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-attachment", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor, attachmentDefinition());

            String response = executeMission(
                    engine,
                    session,
                    attachmentDefinition(),
                    "Extract ticket",
                    Map.of("image", imageResource("ticket.jpg", "SECRET_IMAGE_BYTES")),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.userMediaSeen()).hasSize(2);
        assertThat(chatClient.userMediaSeen()).allSatisfy(media -> assertThat(media.mimeType().toString()).isEqualTo("image/jpeg"));
        assertThat(chatClient.userMessagesSeen()).hasSize(2);
        assertThat(chatClient.userMessagesSeen()).allSatisfy(message -> assertThat(message)
                .contains("\"attachment\" : true", "\"contentType\" : \"image/jpeg\"")
                .doesNotContain("SECRET_IMAGE_BYTES")
                .doesNotContain("ByteArrayResource"));
    }

    @Test
    void retriesWhenModelUsesParentSkillNameInsteadOfBoundReadyTaskTool() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = twoTaskPlan()
                .updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-2","toolName":"duplicateInvoiceChecker","toolArguments":{"payload":"INV-1"}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-2","toolName":"expenseLookup","toolArguments":{"invoiceId":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-parent-skill-tool-confusion", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition("duplicateInvoiceChecker"),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"), tool("expenseLookup", "{\"matches\":[]}")));

            assertThat(response).isEqualTo("Finished");
        }

        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
        assertThat(chatClient.systemMessagesSeen().get(0))
                .contains("CURRENT EXECUTABLE TASK")
                .contains("The only valid toolName for this step is expenseLookup.")
                .doesNotContain("Skill: duplicateInvoiceChecker");
        assertThat(chatClient.userMessagesSeen()).isNotEmpty();
        assertThat(chatClient.userMessagesSeen().get(0)).doesNotContain("duplicateInvoiceChecker");
        assertThat(chatClient.systemMessagesSeen().get(1))
                .contains("Tool 'duplicateInvoiceChecker' is not in the available tools")
                .contains("The only valid toolName for this step is expenseLookup.")
                .contains("Do not call the parent mission skill");
        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.STEP_ACTION_REJECTED
                && String.valueOf(record.metadata().get("reason"))
                .contains("Tool 'duplicateInvoiceChecker' is not in the available tools"));
    }

    @Test
    void retriesFinalResponseUntilRegexLinterPasses() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"APPROVED: Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-linter", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    definitionWithRegexLinter());

            String response = executeMission(
                    engine,
                    session,
                    definitionWithRegexLinter(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("APPROVED: Finished");
        }

        assertThat(session.getLastLinterOutcome()).isPresent();
        assertThat(session.getLastLinterOutcome().orElseThrow().status()).isEqualTo(LinterOutcomeStatus.PASSED);
        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("Must start with APPROVED:");
    }

    @Test
    void honorsConfiguredRegexLinterRetriesAcrossMultipleFinalResponses() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan().updateTask("t-1", task -> task.complete("parsed"));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Finished"}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Still wrong"}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"APPROVED: Finished"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-linter-retries", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    definitionWithRegexLinter(2));

            String response = executeMission(
                    engine,
                    session,
                    definitionWithRegexLinter(2),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}")));

            assertThat(response).isEqualTo("APPROVED: Finished");
        }

        assertThat(session.getLastLinterOutcome()).isPresent();
        assertThat(session.getLastLinterOutcome().orElseThrow().attempt()).isEqualTo(3);
        assertThat(session.getLastLinterOutcome().orElseThrow().maxRetries()).isEqualTo(2);
        assertThat(chatClient.systemMessagesSeen()).hasSize(3);
    }

    @Test
    void rejectsAutoCompletablePlansInStepLoop() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(new PlanTask("t-1", "Summarize results", PlanTaskStatus.PENDING,
                        null, "Summarize mission findings", List.of(), List.of("summary"), true, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-auto", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of()))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("autoCompletable");
        }

        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.ERROR_RECORDED
                && String.valueOf(record.data()).contains("autoCompletable"));
    }

    @Test
    void rejectsPlansWithMissingDependenciesInStepLoop() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-missing-dependency",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(new PlanTask("t-2", "Look up expenses", PlanTaskStatus.PENDING,
                        "expenseLookup", "Find matching expenses", List.of("missing-task"),
                        List.of("expenses"), false, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-missing-dependency", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("expenseLookup", "{\"matches\":[]}"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Missing task dependencies")
                    .hasMessageContaining("t-2->missing-task");
        }

        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.ERROR_RECORDED
                && String.valueOf(record.data()).contains("Missing task dependencies"));
    }

    @Test
    void rejectsPlansWithDuplicateTaskIdsInStepLoop() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-duplicate-task-id",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(
                        new PlanTask("t-1", "Parse invoice A", PlanTaskStatus.PENDING,
                                "invoiceParser", "Parse invoice A", List.of(), List.of("parsedA"), false, null),
                        new PlanTask("t-1", "Parse invoice B", PlanTaskStatus.PENDING,
                                "invoiceParser", "Parse invoice B", List.of(), List.of("parsedB"), false, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-duplicate-task-id", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Task IDs must be unique")
                    .hasMessageContaining("t-1");
        }

        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.ERROR_RECORDED
                && String.valueOf(record.data()).contains("Task IDs must be unique"));
    }

    @Test
    void recordsTerminalFailureWhenInvalidActionRetriesAreExhausted() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Too early"}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Still too early"}
                """);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-invalid-exhausted", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Step action validation exhausted");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED
                && String.valueOf(record.data()).contains("Step action validation exhausted"))).hasSize(1);
        assertThat(records.stream().filter(record -> record.recordType() == TraceRecordType.STEP_FAILED)).hasSize(1);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.STEP_COMPLETED);
    }

    @Test
    void timeoutRecordsOnlyTheCallerOwnedFailureWhenInterruptedModelThrows() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        PlanningService planningService = new InitializingPlanningService(stateService, singleTaskPlan());
        SequenceChatClient chatClient = new SequenceChatClient(BLOCK_UNTIL_INTERRUPTED);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "step-loop-timeout", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService, planningService, missionExecutor, definition(), Duration.ofMillis(100));

            assertThatThrownBy(() -> executeMission(engine, session, definition(), chatClient, List.of()))
                    .isInstanceOf(LoomspanMissionTimeoutException.class)
                    .hasMessageContaining("step-loop-timeout");
        }

        List<TraceRecord> records = readRecords(session);
        assertThat(records.stream()
                .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED))
                .hasSize(1)
                .allSatisfy(record -> assertThat(String.valueOf(record.data()))
                        .contains(LoomspanMissionTimeoutException.class.getName()));
        assertThat(records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED))
                .allSatisfy(record -> assertThat(record.metadata()).containsEntry("status", "aborted"));
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.STEP_FAILED
                || record.recordType() == TraceRecordType.STEP_COMPLETED);
    }

    @Test
    void executesBoundToolCallbacksWithoutRelinkingOrDuplicatePlanUpdates() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(
                        new PlanTask("t-1", "Parse first invoice", PlanTaskStatus.PENDING,
                                "invoiceParser", "Parse invoice A", List.of(), List.of("parsedA"), false, null),
                        new PlanTask("t-2", "Parse second invoice", PlanTaskStatus.PENDING,
                                "invoiceParser", "Parse invoice B", List.of(), List.of("parsedB"), false, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-2","toolName":"invoiceParser","toolArguments":{"rawText":"INV-2"}}
                """,
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":"INV-1"}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Mission complete"}
                """);
        AtomicInteger routerCalls = new AtomicInteger();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-real-tool", "test.entry", 3);
        BoundCapability realWrappedTool = realToolCallback(stateService, planningService, routerCalls, session);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            String response = executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(realWrappedTool));

            assertThat(response).isEqualTo("Mission complete");
        }

        assertThat(routerCalls.get()).isEqualTo(2);
        ExecutionPlan finalPlan = stateService.currentPlan(session).orElseThrow();
        assertThat(finalPlan.findTask("t-1").orElseThrow().status()).isEqualTo(PlanTaskStatus.COMPLETED);
        assertThat(finalPlan.findTask("t-2").orElseThrow().status()).isEqualTo(PlanTaskStatus.COMPLETED);

        long linkedTask2Calls = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.TOOL_CALL_STARTED)
                .filter(record -> "t-2".equals(record.metadata().get("linkedTaskId")))
                .count();
        assertThat(linkedTask2Calls).isEqualTo(1);
    }

    @Test
    void preservesExplicitNullArgumentsInTheStepLoopToolPath()
    {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan("plan-null", "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"), PlanStatus.VALID, null,
                List.of(new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                        "invoiceParser", "Parse invoice", List.of(), List.of("parsed"), false, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient(
                """
                {"stepAction":"CALL_TOOL","taskId":"t-1","toolName":"invoiceParser","toolArguments":{"rawText":null}}
                """,
                """
                {"stepAction":"FINAL_RESPONSE","finalResponse":"Mission complete"}
                """);
        AtomicReference<Map<String, Object>> observedArguments = new AtomicReference<>();
        BoundCapability template = tool("invoiceParser", "unused");
        BoundCapability capability = new BoundCapability(template.metadata(), (arguments, taskId) -> {
            observedArguments.set(arguments);
            return "parsed";
        });
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "step-loop-null", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor())
        {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);
            assertThat(executeMission(engine, session, definition(), chatClient, List.of(capability)))
                    .isEqualTo("Mission complete");
        }

        assertThat(observedArguments.get()).containsEntry("rawText", null);
    }

    @Test
    void rejectsPlansWithTasksMissingCapabilityBindings() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = new ExecutionPlan(
                "plan-unbound-task",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                        null, "Extract invoice data", List.of(), List.of("parsedInvoice"), false, null)));
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient();
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-unbound-capability", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(stateService, planningService, missionExecutor);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    definition(),
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("Tasks missing capability bindings")
                    .hasMessageContaining("t-1");
        }
    }

    @Test
    void rejectsNonPositiveMaxStepsBeforeLoopStarts() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        ExecutionPlan plan = singleTaskPlan();
        PlanningService planningService = new InitializingPlanningService(stateService, plan);
        SequenceChatClient chatClient = new SequenceChatClient();
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        manifest.setMaxSteps(0);
        YamlSkillDefinition invalidDefinition = new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("step-loop-max-steps-zero", "test.entry", 3);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            StepLoopMissionExecutionEngine engine = engine(
                    stateService,
                    planningService,
                    missionExecutor,
                    invalidDefinition);

            assertThatThrownBy(() -> executeMission(
                    engine,
                    session,
                    invalidDefinition,
                    chatClient,
                    List.of(tool("invoiceParser", "{\"vendor\":\"Acme\"}"))))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("max_steps > 0")
                    .hasMessageContaining("was 0");
        }
    }

    private static StepLoopMissionExecutionEngine engine(DefaultExecutionStateService stateService,
                                                         PlanningService planningService,
                                                         ExecutorService missionExecutor) {
        return engine(stateService, planningService, missionExecutor, definition());
    }

    private static String executeMission(StepLoopMissionExecutionEngine engine,
                                         LoomspanSession session,
                                         YamlSkillDefinition definition,
                                         com.lokiscale.loomspan.internal.model.ModelInteraction chatClient,
                                         List<BoundCapability> visibleTools) {
        return executeMission(engine, session, definition, "Check duplicate invoices", null, chatClient, visibleTools);
    }

    private static String executeMission(StepLoopMissionExecutionEngine engine,
                                         LoomspanSession session,
                                         YamlSkillDefinition definition,
                                         String objective,
                                         @Nullable Map<String, Object> missionInput,
                                         com.lokiscale.loomspan.internal.model.ModelInteraction chatClient,
                                         List<BoundCapability> visibleTools) {
        return engine.executeMission(
                session,
                definition,
                objective,
                missionInput,
                chatClient,
                visibleTools,
                true,
                null);
    }

    private static StepLoopMissionExecutionEngine engine(DefaultExecutionStateService stateService,
                                                         PlanningService planningService,
                                                         ExecutorService missionExecutor,
                                                         YamlSkillDefinition definition) {
        return engine(stateService, planningService, missionExecutor, definition, Duration.ofSeconds(5));
    }

    private static StepLoopMissionExecutionEngine engine(DefaultExecutionStateService stateService,
                                                         PlanningService planningService,
                                                         ExecutorService missionExecutor,
                                                         YamlSkillDefinition definition,
                                                         Duration missionTimeout) {
        return new StepLoopMissionExecutionEngine(
                planningService,
                stateService,
                mock(com.lokiscale.loomspan.internal.core.CapabilityRegistry.class),
                new StubYamlSkillCatalog(definition),
                missionTimeout,
                missionExecutor,
                new NoOpSessionUsageService());
    }

    private static YamlSkillDefinition definition() {
        return definition("rootVisibleSkill");
    }

    private static YamlSkillDefinition definition(String name) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName(name);
        manifest.setDescription(name);
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition definitionWithPrompt() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        manifest.setPrompt("STEP_PROMPT_SENTINEL");
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition attachmentDefinition() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        manifest.setInputSchema(attachmentInputSchema());
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillManifest.InputSchemaManifest attachmentInputSchema() {
        YamlSkillManifest.InputSchemaManifest root = new YamlSkillManifest.InputSchemaManifest();
        root.setType("object");
        root.setRequired(List.of("image"));
        root.setAdditionalProperties(false);
        YamlSkillManifest.InputSchemaManifest image = new YamlSkillManifest.InputSchemaManifest();
        image.setType("attachment");
        image.setMediaType("image");
        image.setAllowedContentTypes(List.of("image/jpeg"));
        root.setProperties(Map.of("image", image));
        return root;
    }

    private static ByteArrayResource imageResource(String filename, String content) {
        byte[] marker = content.getBytes(StandardCharsets.UTF_8);
        byte[] bytes = new byte[marker.length + 4];
        bytes[0] = (byte) 0xFF;
        bytes[1] = (byte) 0xD8;
        bytes[2] = (byte) 0xFF;
        System.arraycopy(marker, 0, bytes, 3, marker.length);
        bytes[bytes.length - 1] = (byte) 0xD9;
        return new ByteArrayResource(bytes) {
            @Override
            public String getFilename() {
                return filename;
            }
        };
    }

    private static YamlSkillDefinition definitionWithOutputSchema() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        manifest.setOutputSchemaMaxRetries(1);
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setAdditionalProperties(false);
        YamlSkillManifest.OutputSchemaManifest resultField = new YamlSkillManifest.OutputSchemaManifest();
        resultField.setType("string");
        schema.setProperties(Map.of("result", resultField));
        schema.setRequired(List.of("result"));
        manifest.setOutputSchema(schema);
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition definitionWithOutputSchemaAndEvidenceContract() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        manifest.setOutputSchemaMaxRetries(1);
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setAdditionalProperties(false);
        YamlSkillManifest.OutputSchemaManifest resultField = new YamlSkillManifest.OutputSchemaManifest();
        resultField.setType("string");
        YamlSkillManifest.OutputSchemaManifest reasoningField = new YamlSkillManifest.OutputSchemaManifest();
        reasoningField.setType("string");
        schema.setProperties(Map.of(
                "result", resultField,
                "reasoning", reasoningField));
        schema.setRequired(List.of("result"));
        manifest.setOutputSchema(schema);
        Map<String, String> contract = Map.of(
                "result", "invoiceParser",
                "reasoning", "expenseLookup");
        return new YamlSkillDefinition(
                new ByteArrayResource(new byte[0]),
                manifest,
                EXECUTION_CONFIGURATION,
                com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(contract));
    }

    private static YamlSkillDefinition definitionWithRegexLinter() {
        return definitionWithRegexLinter(1);
    }

    private static YamlSkillDefinition definitionWithRegexLinter(int maxRetries) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPlanningMode(true);
        YamlSkillManifest.LinterManifest linter = new YamlSkillManifest.LinterManifest();
        linter.setType("regex");
        linter.setMaxRetries(maxRetries);
        YamlSkillManifest.RegexManifest regex = new YamlSkillManifest.RegexManifest();
        regex.setPattern("^APPROVED:.*$");
        regex.setMessage("Must start with APPROVED:");
        linter.setRegex(regex);
        manifest.setLinter(linter);
        return new YamlSkillDefinition(new ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static ExecutionPlan singleTaskPlan() {
        return new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                        "invoiceParser", "Extract invoice data", List.of(), List.of("parsedInvoice"), false, null)));
    }

    private static ExecutionPlan twoTaskPlan() {
        return new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                PlanStatus.VALID,
                null,
                List.of(
                        new PlanTask("t-1", "Parse invoice", PlanTaskStatus.PENDING,
                                "invoiceParser", "Extract invoice data", List.of(), List.of("parsedInvoice"), false, null),
                        new PlanTask("t-2", "Look up expenses", PlanTaskStatus.PENDING,
                                "expenseLookup", "Find matching expenses", List.of("t-1"), List.of("expenses"), false, null)));
    }

    private static BoundCapability tool(String name, String result) {
        return new BoundCapability(
                com.lokiscale.loomspan.testkit.TestBoundCapabilities.capability(name).metadata(),
                (arguments, linkedTaskId) -> result);
    }

    private static BoundCapability toolWithSchema(String name, String inputSchema, String result) {
        return new BoundCapability(
                com.lokiscale.loomspan.testkit.TestBoundCapabilities.capability(name, inputSchema).metadata(),
                (arguments, linkedTaskId) -> result);
    }

    private static BoundCapability toolWithContract(String name, String inputSchema, String contractSchema, String result) {
        return new BoundCapability(
                com.lokiscale.loomspan.testkit.TestBoundCapabilities.contractAware(name, inputSchema, contractSchema).metadata(),
                (arguments, linkedTaskId) -> result);
    }

    private static BoundCapability failingTool(String name) {
        return new BoundCapability(
                com.lokiscale.loomspan.testkit.TestBoundCapabilities.capability(name).metadata(),
                (arguments, linkedTaskId) -> { throw new IllegalStateException("parser exploded"); });
    }

    private static BoundCapability realToolCallback(DefaultExecutionStateService stateService,
                                                 PlanningService planningService,
                                                 AtomicInteger routerCalls,
                                                 LoomspanSession session) {
        CapabilityMetadata capability = new CapabilityMetadata(
                "yaml:invoiceParser",
                "invoiceParser",
                "invoiceParser",
                SkillExecutionDescriptor.from(EXECUTION_CONFIGURATION),
                java.util.Set.of(),
                arguments -> "unused",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("invoiceParser", "invoiceParser"),
                null);
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        when(router.execute(eq(capability), any(), eq(session), any())).thenAnswer(invocation -> {
            routerCalls.incrementAndGet();
            @SuppressWarnings("unchecked")
            Map<String, Object> arguments = (Map<String, Object>) invocation.getArgument(1);
            return Map.of("invoice", arguments.get("rawText"));
        });
        DefaultCapabilityInvoker factory = new DefaultCapabilityInvoker(router, planningService, stateService);
        return factory.bind(session, definition(), List.of(capability), null).getFirst();
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static final class InitializingPlanningService implements PlanningService {

        private final DefaultExecutionStateService stateService;
        private final DefaultPlanningService delegate;
        private final ExecutionPlan initialPlan;

        private InitializingPlanningService(DefaultExecutionStateService stateService, ExecutionPlan initialPlan) {
            this.stateService = stateService;
            this.delegate = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
            this.initialPlan = initialPlan;
        }

        @Override
        public Optional<ExecutionPlan> initializePlan(LoomspanSession session,
                                                      String objective,
                                                      @Nullable Map<String, Object> missionInput,
                                                      YamlSkillDefinition definition,
                                                      com.lokiscale.loomspan.internal.model.ModelInteraction chatClient,
                                                      List<BoundCapability> visibleTools) {
            stateService.storePlan(session, initialPlan);
            stateService.logPlanCreated(session, initialPlan, Map.of(
                    "attemptId", "attempt-initial-plan",
                    "retrySequenceId", "retry-initial-plan"));
            return Optional.of(initialPlan);
        }

        @Override
        public Optional<ExecutionPlan> markToolStarted(LoomspanSession session,
                                                       com.lokiscale.loomspan.internal.core.CapabilityMetadata capability,
                                                       Map<String, Object> arguments) {
            throw new UnsupportedOperationException();
        }

        @Override
        public Optional<ExecutionPlan> markTaskStarted(LoomspanSession session,
                                                       String taskId,
                                                       String capabilityName,
                                                       Map<String, Object> arguments) {
            return delegate.markTaskStarted(session, taskId, capabilityName, arguments);
        }

        @Override
        public Optional<ExecutionPlan> markToolCompleted(LoomspanSession session,
                                                         String taskId,
                                                         String capabilityName,
                                                         Object result) {
            return delegate.markToolCompleted(session, taskId, capabilityName, result);
        }

        @Override
        public Optional<ExecutionPlan> markToolFailed(LoomspanSession session,
                                                      String taskId,
                                                      String capabilityName,
                                                      RuntimeException ex) {
            return delegate.markToolFailed(session, taskId, capabilityName, ex);
        }
    }

    private static final class StubYamlSkillCatalog extends YamlSkillCatalog {

        private final YamlSkillDefinition definition;

        private StubYamlSkillCatalog(YamlSkillDefinition definition) {
            super(new com.lokiscale.loomspan.autoconfigure.LoomspanProperties(),
                    new com.lokiscale.loomspan.autoconfigure.LoomspanProperties.Skills());
            this.definition = definition;
        }

        @Override
        public YamlSkillDefinition getSkill(String name) {
            return definition.manifest().getName().equals(name) ? definition : null;
        }
    }

    private static final class SequenceChatClient implements com.lokiscale.loomspan.internal.model.ModelInteraction {

        private final Deque<String> responses = new ArrayDeque<>();
        private final List<String> systemMessagesSeen = new ArrayList<>();
        private final List<String> userMessagesSeen = new ArrayList<>();
        private final List<CapturedMedia> userMediaSeen = new ArrayList<>();

        private SequenceChatClient(String... responses) {
            this.responses.addAll(List.of(responses));
        }

        List<String> systemMessagesSeen() {
            return systemMessagesSeen;
        }

        List<String> userMessagesSeen() {
            return userMessagesSeen;
        }

        List<CapturedMedia> userMediaSeen() {
            return userMediaSeen;
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteractionResult call(
                com.lokiscale.loomspan.internal.model.ModelInteractionRequest request) {
            systemMessagesSeen.add(request.systemPrompt());
            userMessagesSeen.add(request.input().userText());
            request.input().attachments().forEach(attachment -> userMediaSeen.add(
                    new CapturedMedia(MimeType.valueOf(attachment.contentType()), attachment.resource())));
            if (responses.peekFirst() == BLOCK_UNTIL_INTERRUPTED) {
                responses.removeFirst();
                try {
                    new java.util.concurrent.CountDownLatch(1).await();
                }
                catch (InterruptedException ex) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("model interaction interrupted", ex);
                }
            }
            String next = responses.pollFirst();
            if (next == null) {
                throw new IllegalStateException("No more queued chat responses");
            }
            return new com.lokiscale.loomspan.internal.model.ModelInteractionResult(next, Map.of(
                    com.lokiscale.loomspan.internal.core.ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY,
                    request.traceContext().nextAttempt()));
        }

        private record CapturedMedia(MimeType mimeType, Resource resource) {
        }
    }
}
