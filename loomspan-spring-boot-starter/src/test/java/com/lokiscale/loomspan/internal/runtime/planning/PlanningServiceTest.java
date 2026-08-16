package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.JournalEntry;
import com.lokiscale.loomspan.internal.core.JournalEntryType;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.SimpleChatClient;
import com.lokiscale.loomspan.internal.model.ModelInteractionResult;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageValidator;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.definition.ToolDefinition;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.function.Supplier;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class PlanningServiceTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);
    private static final EffectiveSkillExecutionConfiguration EXECUTION_CONFIGURATION =
            new EffectiveSkillExecutionConfiguration("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium");

    private static final String YAML_PLAN_WITH_LLM_STATUSES = """
            ---
            planId: 12345
            capabilityName: invoiceParser
            createdAt: 2023-03-15T14:30:00.000Z
            status: EXECUTED
            activeTaskId: 67890
            tasks:
              - taskId: 67890
                title: Parse Invoice
                status: SUCCESS
                capabilityName: invoiceParser
                intent: Parse the invoice data
                dependsOn: []
                expectedOutputs: []
                autoCompletable: false
                note: Parsed successfully
            """;

    @Test
    void initializesPlanOnlyWhenInvoked() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 3);
        ExecutionPlan plan = plan("plan-1", PlanTaskStatus.PENDING);

        assertThat(session.getExecutionPlan()).isEmpty();
        ExecutionPlan accepted = planningService.initializePlan(
                session, "hello", null, rootDefinition(), new SimpleChatClient(plan, "done"), List.<BoundCapability>of())
                .orElseThrow();
        assertThat(accepted.planId()).isNotEqualTo(plan.planId());
        assertThat(session.getExecutionPlan()).contains(accepted);
    }

    @Test
    void acceptsPlanningResponseWithoutPlanIdAndGeneratesFrameworkIdentity() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-framework-plan-id", "test.entry", 3);
        String response = planJson("model-plan-id", PlanTaskStatus.PENDING)
                .replaceFirst("\\s*\"planId\"\\s*:\\s*\"[^\"]+\"\\s*,", "");

        ExecutionPlan accepted = planningService.initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                request -> new ModelInteractionResult(response, Map.of(
                        ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY,
                        request.traceContext().nextAttempt())),
                List.of())
                .orElseThrow();

        TraceRecord created = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.PLAN_CREATED)
                .findFirst()
                .orElseThrow();
        assertThat(accepted.planId()).isNotBlank();
        assertThat(session.getExecutionPlan()).contains(accepted);
        assertThat(created.metadata().get("planId")).isEqualTo(accepted.planId());
    }

    @Test
    void planCreatedLinksToTheAcceptingAttemptAndRetrySequence() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-plan-lineage", "test.entry", 3);
        Map<String, Object> attempt = Map.of(
                "retrySequenceId", "retry-sequence-accepted",
                "attemptId", "attempt-accepted",
                "attemptNumber", 2,
                "attemptReason", "SEMANTIC_RETRY",
                "providerAttemptNumber", 1);

        planningService.initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                request -> new ModelInteractionResult(planJson("legacy-model-plan-id", PlanTaskStatus.PENDING), Map.of(
                        ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, attempt)),
                List.of());

        TraceRecord created = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.PLAN_CREATED)
                .findFirst()
                .orElseThrow();
        assertThat(created.metadata())
                .containsEntry("attemptId", "attempt-accepted")
                .containsEntry("retrySequenceId", "retry-sequence-accepted")
                .containsEntry("planId", created.data().get("planId").asText());
    }

    @Test
    void planningPromptDoesNotAskTheModelForPlanId() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        SimpleChatClient chatClient = new SimpleChatClient(plan("untrusted-plan", PlanTaskStatus.PENDING), "done");

        planningService.initializePlan(
                com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-prompt-identity", "test.entry", 3),
                "hello", null, rootDefinition(), chatClient, List.of());

        assertThat(chatClient.getSystemMessagesSeen().getFirst())
                .contains("\"taskId\": \"<unique string>\"")
                .doesNotContain("\"planId\"");
    }

    @Test
    void acceptsJsonAndYamlPlansWithoutPlanId() {
        List<String> payloads = List.of(
                planJson("remove-me", PlanTaskStatus.PENDING)
                        .replaceFirst("\\s*\"planId\"\\s*:\\s*\"[^\"]+\"\\s*,", ""),
                YAML_PLAN_WITH_LLM_STATUSES.replaceFirst("(?m)^planId:.*\\R", ""));

        for (int index = 0; index < payloads.size(); index++) {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            String expectedId = "framework-plan-" + index;
            DefaultPlanningService planningService = planningService(stateService, () -> expectedId);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-no-model-id-" + index, "test.entry", 3);

            ExecutionPlan accepted = planningService.initializePlan(
                    session,
                    "hello",
                    null,
                    rootDefinition(index == 0 ? "rootVisibleSkill" : "invoiceParser"),
                    new SimpleChatClient(null, payloads.get(index)),
                    List.of())
                    .orElseThrow();

            assertThat(accepted.planId()).isEqualTo(expectedId);
        }
    }

    @Test
    void overwritesUnsolicitedJsonAndYamlPlanId() {
        List<String> payloads = List.of(
                planJson("adversarial", PlanTaskStatus.PENDING),
                planJson("adversarial", PlanTaskStatus.PENDING).replace("\"adversarial\"", "12345"),
                planJson("adversarial", PlanTaskStatus.PENDING).replace("\"adversarial\"", "\"   \""),
                YAML_PLAN_WITH_LLM_STATUSES,
                YAML_PLAN_WITH_LLM_STATUSES.replace("planId: 12345", "planId: adversarial"),
                YAML_PLAN_WITH_LLM_STATUSES.replace("planId: 12345", "planId: '   '"));

        for (int index = 0; index < payloads.size(); index++) {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            String expectedId = "trusted-plan-" + index;
            ExecutionPlan accepted = planningService(stateService, () -> expectedId).initializePlan(
                    com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                            "session-unsolicited-id-" + index, "test.entry", 3),
                    "hello",
                    null,
                    rootDefinition(index < 3 ? "rootVisibleSkill" : "invoiceParser"),
                    new SimpleChatClient(null, payloads.get(index)),
                    List.of())
                    .orElseThrow();

            assertThat(accepted.planId()).isEqualTo(expectedId);
        }
    }

    @Test
    void rejectsNullOrBlankFrameworkPlanId() {
        List<Supplier<String>> invalidSuppliers = List.of(() -> null, () -> "", () -> "   ");

        for (int index = 0; index < invalidSuppliers.size(); index++) {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            Supplier<String> invalidSupplier = invalidSuppliers.get(index);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-invalid-framework-id-" + index, "test.entry", 3);

            assertThatThrownBy(() -> planningService(stateService, invalidSupplier).initializePlan(
                    session,
                    "hello",
                    null,
                    rootDefinition(),
                    new SimpleChatClient(plan("untrusted", PlanTaskStatus.PENDING), "done"),
                    List.of()))
                    .isInstanceOfAny(NullPointerException.class, IllegalArgumentException.class);
            assertThat(session.getExecutionPlan()).isEmpty();
            assertThat(readRecords(session)).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_CREATED);
        }
    }

    @Test
    void identicalAcceptedResponsesReceiveDistinctFrameworkPlanIds() {
        Deque<String> ids = new ArrayDeque<>(List.of("framework-plan-1", "framework-plan-2"));
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = planningService(stateService, ids::removeFirst);
        String response = planJson("same-untrusted-id", PlanTaskStatus.PENDING);

        ExecutionPlan first = planningService.initializePlan(
                com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-distinct-1", "test.entry", 3),
                "hello", null, rootDefinition(), new SimpleChatClient(null, response), List.of()).orElseThrow();
        ExecutionPlan second = planningService.initializePlan(
                com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-distinct-2", "test.entry", 3),
                "hello", null, rootDefinition(), new SimpleChatClient(null, response), List.of()).orElseThrow();

        assertThat(first.planId()).isEqualTo("framework-plan-1");
        assertThat(second.planId()).isEqualTo("framework-plan-2");
    }

    @Test
    void missingAcceptedAttemptContextFailsBeforePlanStorage() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "session-missing-attempt", "test.entry", 3);

        assertThatThrownBy(() -> planningService(stateService, () -> "candidate-plan").initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                request -> ModelInteractionResult.content(planJson("untrusted", PlanTaskStatus.PENDING)),
                List.of()))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("model attempt context");
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(readRecords(session)).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_CREATED);
    }

    @Test
    void invalidAcceptedAttemptContextFailsBeforePlanStorage() {
        List<Map<String, Object>> invalidContexts = List.of(
                Map.of(ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, "not-a-map"),
                Map.of(ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, Map.of(
                        "retrySequenceId", "retry-invalid",
                        "attemptNumber", 1,
                        "attemptReason", "INITIAL",
                        "providerAttemptNumber", 1)),
                Map.of(ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, Map.of(
                        "attemptId", "attempt-invalid",
                        "attemptNumber", 1,
                        "attemptReason", "INITIAL",
                        "providerAttemptNumber", 1)),
                Map.of(ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY, Map.of(
                        "retrySequenceId", "retry-invalid",
                        "attemptId", " ",
                        "attemptNumber", 1,
                        "attemptReason", "INITIAL",
                        "providerAttemptNumber", 1)));

        for (int index = 0; index < invalidContexts.size(); index++) {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-invalid-attempt-" + index, "test.entry", 3);
            Map<String, Object> invalidContext = invalidContexts.get(index);

            assertThatThrownBy(() -> planningService(stateService, () -> "candidate-plan").initializePlan(
                    session,
                    "hello",
                    null,
                    rootDefinition(),
                    request -> new ModelInteractionResult(planJson("untrusted", PlanTaskStatus.PENDING), invalidContext),
                    List.of()))
                    .isInstanceOf(RuntimeException.class);
            assertThat(session.getExecutionPlan()).isEmpty();
            assertThat(readRecords(session)).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_CREATED);
        }
    }

    @Test
    void planningReceivesAttachmentDescriptorsButNoMedia() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-planning-attachment", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(planJson("plan-attachment", PlanTaskStatus.PENDING));

        planningService.initializePlan(
                session,
                "Extract ticket",
                Map.of("image", Map.of(
                        "attachment", true,
                        "name", "ticket.jpg",
                        "contentType", "image/jpeg",
                        "mediaType", "IMAGE")),
                rootDefinition(),
                chatClient,
                List.of());

        assertThat(chatClient.userMessagesSeen()).hasSize(1);
        assertThat(chatClient.userMessagesSeen().getFirst()).contains("\"attachment\" : true", "\"contentType\" : \"image/jpeg\"");
        assertThat(chatClient.userConsumerCalls()).isZero();
    }

    @Test
    void doesNotPerformDuplicateOuterPlanningUsageAccounting() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        RecordingSessionUsageService usageService = new RecordingSessionUsageService();
        DefaultPlanningService planningService = new DefaultPlanningService(
                new DefaultPlanTaskLinker(),
                stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-usage", "test.entry", 3);
        ExecutionPlan plan = plan("plan-usage", PlanTaskStatus.PENDING);

        assertThat(planningService.initializePlan(session, "hello", null, rootDefinition(), new SimpleChatClient(plan, "done"), List.of()))
                .isPresent();
        assertThat(usageService.lastSkillName).isNull();
        assertThat(usageService.snapshot(session).modelCalls()).isZero();
    }

    @Test
    void initializesPlanFromYamlWithNormalizedStatuses() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-yaml", "test.entry", 3);

        ExecutionPlan plan = planningService.initializePlan(
                        session,
                        "parse invoice",
                        null,
                        rootDefinition("invoiceParser"),
                        new SimpleChatClient(null, YAML_PLAN_WITH_LLM_STATUSES),
                        List.<BoundCapability>of())
                .orElseThrow();

        assertThat(plan.planId()).isNotBlank().isNotEqualTo("12345");
        assertThat(plan.status()).isEqualTo(PlanStatus.VALID);
        assertThat(plan.findTask("67890")).isPresent();
        assertThat(plan.findTask("67890").orElseThrow().status()).isEqualTo(PlanTaskStatus.COMPLETED);
    }

    @Test
    void planningCodecRoleRejectsUnknownFieldsInJsonAndYaml()
    {
        for (String payload : List.of(
                planJson("plan-json-unknown", PlanTaskStatus.PENDING)
                        .replaceFirst("\\{", "{\"future\":true,"),
                YAML_PLAN_WITH_LLM_STATUSES + "future: true\n"))
        {
            DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
            DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-plan-unknown-" + payload.charAt(0), "test.entry", 3);

            assertThatThrownBy(() -> planningService.initializePlan(
                    session,
                    "parse invoice",
                    null,
                    rootDefinition(payload.startsWith("{") ? "rootVisibleSkill" : "invoiceParser"),
                    new SimpleChatClient(null, payload),
                    List.<BoundCapability>of()))
                    .hasMessageContaining("Failed to parse planning response");
        }
    }

    @Test
    void allowsLegacyPlanningResponsesWithoutStepLoopContract() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-legacy-plan", "test.entry", 3);

        ExecutionPlan legacyPlan = new ExecutionPlan(
                "plan-legacy",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Summarize", PlanTaskStatus.PENDING,
                        null, "Summarize results", List.of(), List.of("summary"), true, null)));

        assertThat(planningService.initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                new SimpleChatClient(legacyPlan, "done"),
                List.of()))
                .isPresent();
        assertThat(session.getExecutionPlan()).isPresent();
    }

    @Test
    void recordsPlanningTraceWithRealProviderMetadata() throws Exception {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-trace", "test.entry", 3);

        planningService.initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                new SimpleChatClient(plan("plan-trace", PlanTaskStatus.PENDING), "done"),
                List.of());

        List<TraceRecord> records = readRecords(session);

        TraceRecord planningFrame = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_OPENED
                        && record.frameType() == TraceFrameType.PLANNING)
                .findFirst()
                .orElseThrow();
        TraceRecord modelFrame = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.FRAME_OPENED
                        && record.frameType() == TraceFrameType.MODEL_CALL
                        && "rootVisibleSkill#planning-model".equals(record.route()))
                .findFirst()
                .orElseThrow();

        assertThat(modelFrame.parentFrameId()).isEqualTo(planningFrame.frameId());
        TraceRecord created = records.stream()
                .filter(record -> record.recordType() == TraceRecordType.PLAN_CREATED)
                .findFirst()
                .orElseThrow();
        assertThat(created.metadata()).containsKeys("planId", "attemptId", "retrySequenceId");
        assertThat(records).noneMatch(record ->
                record.recordType() == TraceRecordType.MODEL_REQUEST_SENT);
    }

    @Test
    void marksLinkedTaskStartedCompletedAndBlocked() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 3);
        CapabilityMetadata capability = capability("allowedVisibleSkill");

        stateService.storePlan(session, new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Use tool", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null),
                        new PlanTask("task-2", "Summarize", PlanTaskStatus.PENDING, null))));

        ExecutionPlan started = planningService.markToolStarted(session, capability, Map.of("value", "hello")).orElseThrow();
        assertThat(started.activeTaskId()).isEqualTo("task-1");
        assertThat(started.findTask("task-1").orElseThrow().status()).isEqualTo(PlanTaskStatus.IN_PROGRESS);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(JournalEntryType.PLAN_UPDATED);

        ExecutionPlan completed = planningService.markToolCompleted(session, "task-1", capability.name(), "done").orElseThrow();
        assertThat(completed.activeTaskId()).isNull();
        assertThat(completed.findTask("task-1").orElseThrow().status()).isEqualTo(PlanTaskStatus.COMPLETED);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-2",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:01:00Z"),
                List.of(new PlanTask("task-3", "Use tool", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null))));
        ExecutionPlan blocked = planningService.markToolStarted(session, capability, Map.of()).orElseThrow();
        assertThat(planningService.markToolFailed(session, blocked.activeTaskId(), capability.name(), new IllegalStateException("boom")))
                .isPresent()
                .get()
                .extracting(ExecutionPlan::status)
                .isEqualTo(PlanStatus.STALE);
        assertThat(session.getExecutionPlan().orElseThrow().findTask("task-3").orElseThrow().status()).isEqualTo(PlanTaskStatus.BLOCKED);
        assertThat(session.getJournalSnapshot()).extracting(JournalEntry::type)
                .containsExactly(
                        JournalEntryType.PLAN_UPDATED,
                        JournalEntryType.PLAN_UPDATED,
                        JournalEntryType.PLAN_UPDATED,
                        JournalEntryType.PLAN_UPDATED);
    }

    @Test
    void marksExplicitTaskStartedWithoutRelinking() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-explicit-task", "test.entry", 3);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-explicit",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(
                        new PlanTask("task-1", "Use tool once", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null),
                        new PlanTask("task-2", "Use tool twice", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool again", List.of(), List.of(), false, null))));

        ExecutionPlan started = planningService.markTaskStarted(session, "task-2", "allowedVisibleSkill", Map.of("value", "hello"))
                .orElseThrow();

        assertThat(started.activeTaskId()).isEqualTo("task-2");
        assertThat(started.findTask("task-2").orElseThrow().status()).isEqualTo(PlanTaskStatus.IN_PROGRESS);
        assertThat(started.findTask("task-1").orElseThrow().status()).isEqualTo(PlanTaskStatus.PENDING);
    }

    @Test
    void rejectsStartingTaskWithMismatchedCapabilityBinding() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-mismatched-start", "test.entry", 3);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-explicit",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool once", PlanTaskStatus.PENDING,
                        "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null))));

        assertThatThrownBy(() -> planningService.markTaskStarted(session, "task-1", "different.visible.skill", Map.of()))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("task-1")
                .hasMessageContaining("allowedVisibleSkill")
                .hasMessageContaining("different.visible.skill");
    }

    @Test
    void rejectsCompletingTaskWithMismatchedCapabilityBinding() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-mismatched-complete", "test.entry", 3);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-explicit",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool once", PlanTaskStatus.IN_PROGRESS,
                        "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null))));

        assertThatThrownBy(() -> planningService.markToolCompleted(session, "task-1", "different.visible.skill", "done"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("task-1")
                .hasMessageContaining("allowedVisibleSkill")
                .hasMessageContaining("different.visible.skill");
    }

    @Test
    void rejectsFailingTaskWithMismatchedCapabilityBinding() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-mismatched-fail", "test.entry", 3);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-explicit",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool once", PlanTaskStatus.IN_PROGRESS,
                        "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null))));

        assertThatThrownBy(() -> planningService.markToolFailed(
                session,
                "task-1",
                "different.visible.skill",
                new IllegalStateException("boom")))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("task-1")
                .hasMessageContaining("allowedVisibleSkill")
                .hasMessageContaining("different.visible.skill");
    }

    @Test
    void doesNotLogPlanUpdateWhenCompletedTaskIsMissing() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-missing-task", "test.entry", 3);

        stateService.storePlan(session, new ExecutionPlan(
                "plan-1",
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null))));

        assertThat(planningService.markToolCompleted(session, "missing-task", "allowedVisibleSkill", "done")).isEmpty();
        assertThat(session.getJournalSnapshot()).isEmpty();
    }

    @Test
    void planningPromptIncludesToolDescriptionsAndAlignmentRules() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-tool-names", "test.entry", 3);
        SimpleChatClient chatClient = new SimpleChatClient(plan("plan-tools", PlanTaskStatus.PENDING), "done");

        BoundCapability tool1 = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability tool2 = toolCallback("expenseLookup", "Look up prior expenses for a parsed invoice");

        planningService.initializePlan(session, "check invoice", null, rootDefinition("duplicateInvoiceChecker"), chatClient, List.of(tool1, tool2));

        String systemPrompt = chatClient.getSystemMessagesSeen().getFirst();
        assertThat(systemPrompt).contains("invoiceParser: Extract invoice fields from source documents");
        assertThat(systemPrompt).contains("expenseLookup: Look up prior expenses for a parsed invoice");
        assertThat(systemPrompt).contains("Available sub-skills");
        assertThat(systemPrompt).contains("Bind each task to the tool that best matches that task's intent.");
        assertThat(systemPrompt).contains("Gather enough evidence to support the final answer before the mission is complete.");
        assertThat(systemPrompt).contains("\"capabilityName\": \"duplicateInvoiceChecker\"");
    }

    @Test
    void planningPromptIncludesSkillPromptBeforePlanningContract() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-planning-prompt", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(planJson("plan-prompt", PlanTaskStatus.PENDING));

        BoundCapability tool = toolCallback("invoiceParser", "Short child tool description");
        planningService.initializePlan(
                session,
                "check invoice",
                null,
                rootDefinitionWithPrompt("PARENT_PROMPT_SENTINEL\nAlways verify totals before final response."),
                chatClient,
                List.of(tool));

        String systemPrompt = chatClient.systemMessagesSeen().getFirst();
        assertThat(systemPrompt).startsWith("PARENT_PROMPT_SENTINEL");
        assertThat(systemPrompt.indexOf("PARENT_PROMPT_SENTINEL"))
                .isLessThan(systemPrompt.indexOf("Create an ordered flight plan"));
        assertThat(systemPrompt).contains("invoiceParser: Short child tool description");
        assertThat(systemPrompt).doesNotContain("invoiceParser: PARENT_PROMPT_SENTINEL");

        assertThat(readRecords(session))
                .noneMatch(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_PREPARED);
    }

    @Test
    void planningPromptIncludesEvidenceConstraints() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-evidence-constraints", "test.entry", 3);
        SimpleChatClient chatClient = new SimpleChatClient(plan("plan-tools", PlanTaskStatus.PENDING), "done");

        BoundCapability tool1 = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability tool2 = toolCallback("expenseLookup", "Look up prior expenses for a parsed invoice");

        assertThatThrownBy(() -> planningService.initializePlan(session, "check invoice", null, duplicateInvoiceDefinition(), chatClient, List.of(tool1, tool2)))
                .isInstanceOf(IllegalStateException.class);

        String systemPrompt = chatClient.getSystemMessagesSeen().getFirst();
        assertThat(systemPrompt).contains("Evidence Constraints:");
        assertThat(systemPrompt).contains("The 'isDuplicate' output field requires tasks whose exact capability names satisfy: invoiceParser and expenseLookup");
        assertThat(systemPrompt).contains("The 'vendorName' output field requires tasks whose exact capability names satisfy: invoiceParser");
        assertThat(systemPrompt).doesNotContain("[expenseLookup, invoiceParser] tool(s)");
    }

    @Test
    void planningPromptPreservesAuthoredDescriptionsVerbatim() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-authored-descriptions", "test.entry", 3);
        SimpleChatClient chatClient = new SimpleChatClient(plan("plan-tools-verbatim", PlanTaskStatus.PENDING), "done");

        String authoredDescription = "Reads invoice PDFs exactly as-authored. Keep JSON keys `invoice_id`, `vendor_name`, and \"line_items\".";
        BoundCapability tool = toolCallback("invoiceParser", authoredDescription);

        planningService.initializePlan(
                session,
                "check invoice",
                null,
                rootDefinition("duplicateInvoiceChecker"),
                chatClient,
                List.of(tool));

        String systemPrompt = chatClient.getSystemMessagesSeen().getFirst();
        assertThat(systemPrompt).contains("invoiceParser: " + authoredDescription);
    }

    @Test
    void retriesSingleToolOverusePlanWhenMultipleVisibleToolsExist() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-weak-plan-retry", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(
                weakSingleToolPlanJson(),
                correctedMultiToolPlanJson());

        BoundCapability invoiceParser = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability expenseLookup = toolCallback("expenseLookup", "Look up related expenses for comparison");

        ExecutionPlan plan = planningService.initializePlan(
                        session,
                        "check invoice duplicates",
                        null,
                        rootDefinition("duplicateInvoiceChecker"),
                        chatClient,
                        List.of(invoiceParser, expenseLookup))
                .orElseThrow();

        assertThat(plan.tasks()).extracting(PlanTask::capabilityName)
                .containsExactly("invoiceParser", "expenseLookup", "expenseLookup");
        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("Previous plan was too weak");
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("overuses 'invoiceParser'");
        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.PLAN_VALIDATION_FAILED);
        assertThat(readRecords(session)).anyMatch(record -> record.recordType() == TraceRecordType.PLAN_RETRY_REQUESTED);
    }

    @Test
    void leavesPlanningRetryUsageToThePhysicalAttemptAdvisor() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        RecordingSessionUsageService usageService = new RecordingSessionUsageService();
        DefaultPlanningService planningService = new DefaultPlanningService(
                new DefaultPlanTaskLinker(),
                stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-weak-plan-retry-usage", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(
                weakSingleToolPlanJson(),
                correctedMultiToolPlanJson());

        BoundCapability invoiceParser = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability expenseLookup = toolCallback("expenseLookup", "Look up related expenses for comparison");

        planningService.initializePlan(
                        session,
                        "check invoice duplicates",
                        null,
                        rootDefinition("duplicateInvoiceChecker"),
                        chatClient,
                        List.of(invoiceParser, expenseLookup))
                .orElseThrow();

        assertThat(usageService.lastSkillName).isNull();
        assertThat(usageService.snapshot(session).modelCalls()).isZero();
    }

    @Test
    void stopsRetryingAfterConfiguredPlanQualityRetryCap() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        RecordingSessionUsageService usageService = new RecordingSessionUsageService();
        DefaultPlanningService planningService = new DefaultPlanningService(
                new DefaultPlanTaskLinker(),
                stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-weak-plan-retry-cap", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(
                weakSingleToolPlanJson(),
                weakSingleToolPlanJson());

        BoundCapability invoiceParser = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability expenseLookup = toolCallback("expenseLookup", "Look up related expenses for comparison");

        ExecutionPlan plan = planningService.initializePlan(
                        session,
                        "check invoice duplicates",
                        null,
                        rootDefinition("duplicateInvoiceChecker"),
                        chatClient,
                        List.of(invoiceParser, expenseLookup))
                .orElseThrow();

        assertThat(plan.tasks()).extracting(PlanTask::capabilityName)
                .containsExactly("invoiceParser", "invoiceParser", "invoiceParser");
        assertThat(chatClient.systemMessagesSeen()).hasSize(2);
        assertThat(chatClient.systemMessagesSeen().get(1)).contains("Previous plan was too weak");
        assertThat(usageService.snapshot(session).modelCalls()).isZero();

        List<TraceRecord> records = readRecords(session);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.PLAN_VALIDATION_FAILED)
                .hasSize(1);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.PLAN_RETRY_REQUESTED)
                .hasSize(1);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.PLAN_QUALITY_WARNING)
                .hasSize(2);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.PLAN_QUALITY_WARNING)
                .filteredOn(record -> "ERROR".equals(record.metadata().get("severity")))
                .hasSize(1)
                .first()
                .satisfies(record -> {
                    assertThat(record.frameType()).isEqualTo(TraceFrameType.PLANNING);
                    assertThat(record.metadata()).containsEntry("retryCount", 1);
                    assertThat(record.metadata()).containsEntry("severity", "ERROR");
                });
    }

    @Test
    void warnsButAcceptsLegitimateRepeatedToolPlan() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-repeated-tool-warning", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(repeatedExtractionPlanJson());

        BoundCapability invoiceParser = toolCallback("invoiceParser", "Extract invoice fields from source documents");
        BoundCapability expenseLookup = toolCallback("expenseLookup", "Look up related expenses for comparison");

        ExecutionPlan plan = planningService.initializePlan(
                        session,
                        "extract invoice details",
                        null,
                        rootDefinition("duplicateInvoiceChecker"),
                        chatClient,
                        List.of(invoiceParser, expenseLookup))
                .orElseThrow();

        assertThat(plan.tasks()).extracting(PlanTask::capabilityName)
                .containsExactly("invoiceParser", "invoiceParser", "invoiceParser");
        assertThat(chatClient.systemMessagesSeen()).hasSize(1);
        List<TraceRecord> records = readRecords(session);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_VALIDATION_FAILED);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_RETRY_REQUESTED);
        assertThat(records).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_QUALITY_WARNING);
    }

    @Test
    void ignoresSemanticMismatchHeuristicsWhenToolMetadataIsSparse() {
        PlanQualityValidator validator = new PlanQualityValidator();
        ExecutionPlan plan = new ExecutionPlan(
                "plan-sparse-metadata",
                "duplicateInvoiceChecker",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask(
                        "task-1",
                        "Final report",
                        PlanTaskStatus.PENDING,
                        "toolA",
                        "Summarize the result",
                        List.of(),
                        List.of("report"),
                        false,
                        "")));

        PlanQualityValidationResult validation = validator.validate(
                plan,
                List.of(
                        toolCallback("toolA", "Helper tool"),
                        toolCallback("toolB", "")));

        assertThat(validation.warnings()).isEmpty();
        assertThat(validation.errors()).isEmpty();
    }

    @Test
    void planningPromptShowsNoneWhenNoToolsProvided() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-no-tools", "test.entry", 3);
        SimpleChatClient chatClient = new SimpleChatClient(plan("plan-no-tools", PlanTaskStatus.PENDING), "done");

        planningService.initializePlan(session, "check invoice", null, rootDefinition("duplicateInvoiceChecker"), chatClient, List.of());

        String systemPrompt = chatClient.getSystemMessagesSeen().getFirst();
        assertThat(systemPrompt).contains("(none)");
    }

    @Test
    void planningPromptHardcodesTopLevelCapabilityName() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-cap-name", "test.entry", 3);
        SimpleChatClient chatClient = new SimpleChatClient(plan("plan-cap", PlanTaskStatus.PENDING), "done");

        planningService.initializePlan(session, "check invoice", null, rootDefinition("duplicateInvoiceChecker"), chatClient, List.of());

        String systemPrompt = chatClient.getSystemMessagesSeen().getFirst();
        assertThat(systemPrompt).contains("\"capabilityName\": \"duplicateInvoiceChecker\"");
    }

    @Test
    void rejectsContractBackedPlanWhenRequiredEvidenceRemainsUncoveredAfterRetries() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-evidence-fail", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(weakSingleToolPlanJson(), weakSingleToolPlanJson());

        assertThatThrownBy(() -> planningService.initializePlan(
                session,
                "check duplicate invoice",
                null,
                duplicateInvoiceDefinition(),
                chatClient,
                List.of(toolCallback("invoiceParser", "Extract invoice fields"), toolCallback("expenseLookup", "Look up matching expenses"))))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Evidence coverage validation failed");
        assertThat(session.getExecutionPlan()).isEmpty();
        assertThat(readRecords(session)).noneMatch(record -> record.recordType() == TraceRecordType.PLAN_CREATED);
    }

    @Test
    void acceptsContractBackedPlanWhenTaskBindingsCoverAllRequiredEvidence() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-evidence-pass", "test.entry", 3);
        SequencePlanningChatClient chatClient = new SequencePlanningChatClient(correctedMultiToolPlanJson());

        ExecutionPlan plan = planningService.initializePlan(
                        session,
                        "check duplicate invoice",
                        null,
                        duplicateInvoiceDefinition(),
                        chatClient,
                        List.of(toolCallback("invoiceParser", "Extract invoice fields"), toolCallback("expenseLookup", "Look up matching expenses")))
                .orElseThrow();

        assertThat(plan.tasks()).extracting(PlanTask::capabilityName)
                .contains("invoiceParser", "expenseLookup");
    }

    private static BoundCapability toolCallback(String name, String description) {
        return com.lokiscale.loomspan.testkit.TestBoundCapabilities.describedCapability(name, description);
    }

    private static String weakSingleToolPlanJson() {
        return """
                {
                  "planId": "plan-weak",
                  "capabilityName": "duplicateInvoiceChecker",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {
                      "taskId": "t-1",
                      "title": "Parse invoice",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Extract invoice fields",
                      "dependsOn": [],
                      "expectedOutputs": ["parsed invoice"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-2",
                      "title": "Check duplicates",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Check the invoice against prior expenses",
                      "dependsOn": ["t-1"],
                      "expectedOutputs": ["duplicate matches"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-3",
                      "title": "Final report",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Summarize the duplicate invoice result",
                      "dependsOn": ["t-2"],
                      "expectedOutputs": ["final report"],
                      "autoCompletable": false,
                      "note": ""
                    }
                  ]
                }
                """;
    }

    private static String correctedMultiToolPlanJson() {
        return """
                {
                  "planId": "plan-strong",
                  "capabilityName": "duplicateInvoiceChecker",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {
                      "taskId": "t-1",
                      "title": "Parse invoice",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Extract invoice fields",
                      "dependsOn": [],
                      "expectedOutputs": ["parsed invoice"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-2",
                      "title": "Look up matching expenses",
                      "status": "PENDING",
                      "capabilityName": "expenseLookup",
                      "intent": "Find matching expenses for the parsed invoice",
                      "dependsOn": ["t-1"],
                      "expectedOutputs": ["matching expenses"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-3",
                      "title": "Compare evidence",
                      "status": "PENDING",
                      "capabilityName": "expenseLookup",
                      "intent": "Compare the parsed invoice against matching expenses",
                      "dependsOn": ["t-2"],
                      "expectedOutputs": ["duplicate decision"],
                      "autoCompletable": false,
                      "note": ""
                    }
                  ]
                }
                """;
    }

    private static String repeatedExtractionPlanJson() {
        return """
                {
                  "planId": "plan-repeat",
                  "capabilityName": "duplicateInvoiceChecker",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {
                      "taskId": "t-1",
                      "title": "Extract invoice header",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Extract invoice number and vendor",
                      "dependsOn": [],
                      "expectedOutputs": ["invoice header"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-2",
                      "title": "Extract invoice line items",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Extract invoice line items",
                      "dependsOn": ["t-1"],
                      "expectedOutputs": ["line items"],
                      "autoCompletable": false,
                      "note": ""
                    },
                    {
                      "taskId": "t-3",
                      "title": "Extract tax details",
                      "status": "PENDING",
                      "capabilityName": "invoiceParser",
                      "intent": "Extract invoice tax details",
                      "dependsOn": ["t-2"],
                      "expectedOutputs": ["tax details"],
                      "autoCompletable": false,
                      "note": ""
                    }
                  ]
                }
                """;
    }

    private static CapabilityMetadata capability(String name) {
        return new CapabilityMetadata(
                "yaml:child",
                name,
                "child",
                SkillExecutionDescriptor.from(new com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration(
                        "gpt-5",
                        "test-connection", AiDriver.OPENAI,
                        "openai/gpt-5",
                        "medium")),
                java.util.Set.of(),
                arguments -> "ok",
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic(name, "child"),
                "targetBean#deterministicTarget");
    }

    private static YamlSkillDefinition duplicateInvoiceDefinition() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("duplicateInvoiceChecker");
        manifest.setDescription("duplicateInvoiceChecker");
        manifest.setModel("gpt-5");
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType("object");
        schema.setProperties(Map.of(
                "vendorName", scalarSchema("string"),
                "isDuplicate", scalarSchema("boolean")));
        schema.setRequired(List.of("vendorName", "isDuplicate"));
        schema.setAdditionalProperties(false);
        manifest.setOutputSchema(schema);
        manifest.setOutputSchemaMaxRetries(1);
        Map<String, String> contract = Map.of(
                "vendorName", "invoiceParser",
                "isDuplicate", "invoiceParser and expenseLookup");
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                EXECUTION_CONFIGURATION,
                com.lokiscale.loomspan.internal.runtime.evidence.TestEvidenceContracts.compiled(contract));
    }

    private static YamlSkillDefinition rootDefinition() {
        return rootDefinition("rootVisibleSkill");
    }

    private static YamlSkillDefinition rootDefinition(String name) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName(name);
        manifest.setDescription(name);
        manifest.setModel("gpt-5");
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition rootDefinitionWithPrompt(String prompt) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("Short planner-facing summary");
        manifest.setModel("gpt-5");
        manifest.setPrompt(prompt);
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                EXECUTION_CONFIGURATION);
    }

    private static YamlSkillManifest.OutputSchemaManifest scalarSchema(String type) {
        YamlSkillManifest.OutputSchemaManifest schema = new YamlSkillManifest.OutputSchemaManifest();
        schema.setType(type);
        return schema;
    }

    private static ExecutionPlan plan(String id, PlanTaskStatus status) {
        return new ExecutionPlan(
                id,
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of(new PlanTask("task-1", "Use tool", status,
                        "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null)));
    }

    private static DefaultPlanningService planningService(
            DefaultExecutionStateService stateService,
            Supplier<String> planIdSupplier) {
        LoomspanJacksonCodecs codecs = LoomspanJacksonCodecs.defaults();
        return new DefaultPlanningService(
                new DefaultPlanTaskLinker(),
                stateService,
                codecs.planningJson(),
                codecs.planningYaml(),
                new PlanQualityValidator(),
                new EvidenceCoverageValidator(),
                planIdSupplier);
    }

    private static String planJson(String id, PlanTaskStatus status) {
        return """
                {
                  "planId": "%s",
                  "capabilityName": "rootVisibleSkill",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "tasks": [
                    {
                      "taskId": "task-1",
                      "title": "Use tool",
                      "status": "%s",
                      "capabilityName": "allowedVisibleSkill",
                      "intent": "Use tool",
                      "dependsOn": [],
                      "expectedOutputs": [],
                      "autoCompletable": false
                    }
                  ]
                }
                """.formatted(id, status.name());
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static final class RecordingSessionUsageService implements SessionUsageService {

        private String lastSkillName;

        @Override
        public SessionUsageSnapshot snapshot(LoomspanSession session) {
            return session.getSessionUsage().orElse(SessionUsageSnapshot.empty());
        }

        @Override
        public void recordMissionStart(LoomspanSession session, String skillName) {
        }

        @Override
        public void reserveProviderAttempt(LoomspanSession session, String skillName) {
        }

        @Override
        public void recordProviderAttemptOutcome(String skillName,
                com.lokiscale.loomspan.internal.core.ModelExecutionIdentity identity, String outcome,
                com.lokiscale.loomspan.internal.provider.ProviderFailureCategory category,
                com.lokiscale.loomspan.internal.provider.ProviderRetryDecision decision) {
        }

        @Override
        public void recordModelResponse(LoomspanSession session,
                                        String skillName,
                                        com.lokiscale.loomspan.internal.core.ModelExecutionIdentity identity,
                                        com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord usageRecord) {
            lastSkillName = skillName;
            SessionUsageSnapshot existing = snapshot(session);
            session.setSessionUsage(existing.recordModelUsage(usageRecord));
        }

        @Override
        public void recordToolCall(LoomspanSession session, String skillName, String capabilityName) {
        }

        @Override
        public void recordLinterOutcome(LoomspanSession session, com.lokiscale.loomspan.internal.linter.LinterOutcome outcome) {
        }
    }

    private static final class SequencePlanningChatClient implements com.lokiscale.loomspan.internal.model.ModelInteraction {

        private final Deque<String> responses = new ArrayDeque<>();
        private final List<String> systemMessagesSeen = new ArrayList<>();
        private final List<String> userMessagesSeen = new ArrayList<>();
        private int userConsumerCalls;

        private SequencePlanningChatClient(String... responses) {
            this.responses.addAll(List.of(responses));
        }

        private List<String> systemMessagesSeen() {
            return systemMessagesSeen;
        }

        private List<String> userMessagesSeen() {
            return userMessagesSeen;
        }

        private int userConsumerCalls() {
            return userConsumerCalls;
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteractionResult call(
                com.lokiscale.loomspan.internal.model.ModelInteractionRequest request) {
            systemMessagesSeen.add(request.systemPrompt());
            userMessagesSeen.add(request.input().userText());
            if (!request.input().attachments().isEmpty()) {
                userConsumerCalls++;
            }
            String next = responses.pollFirst();
            if (next == null) {
                throw new IllegalStateException("No more queued chat responses");
            }
            return new com.lokiscale.loomspan.internal.model.ModelInteractionResult(next, Map.of(
                    ModelTraceContext.RESPONSE_ATTEMPT_CONTEXT_KEY,
                    request.traceContext().nextAttempt()));
        }
    }
}
