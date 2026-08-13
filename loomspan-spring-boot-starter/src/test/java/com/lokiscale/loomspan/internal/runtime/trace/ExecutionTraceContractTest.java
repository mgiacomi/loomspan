package com.lokiscale.loomspan.internal.runtime.trace;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.SimpleChatClient;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.junit.jupiter.api.Test;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.ai.tool.definition.ToolDefinition;

import java.util.ArrayDeque;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;
import java.util.concurrent.Executors;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ExecutionTraceContractTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);
    private static final EffectiveSkillExecutionConfiguration EXECUTION_CONFIGURATION =
            new EffectiveSkillExecutionConfiguration("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium");

    @Test
    void engineCallSitesDoNotEmitDuplicateOuterModelEvents() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);

        LoomspanSession planningSession = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("planning-trace", "test.entry", 3);
        planningService.initializePlan(
                planningSession,
                "hello",
                null,
                rootDefinition(),
                new SimpleChatClient(plan("plan-1"), "done"),
                List.<BoundCapability>of());

        List<TraceRecord> planningModelRecords = modelRecords(planningSession);

        LoomspanSession missionSession = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("mission-trace", "test.entry", 3);
        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService),
                    stateService,
                    Duration.ofSeconds(5),
                    executor);

            String missionResponse = engine.executeMission(
                    missionSession,
                    rootDefinition(),
                    "hello",
                    null,
                    new SimpleChatClient(null, "mission complete"),
                    List.of(),
                    false,
                    null);

            assertThat(missionResponse).isEqualTo("mission complete");
        }

        List<TraceRecord> missionModelRecords = modelRecords(missionSession);

        assertThat(planningModelRecords).isEmpty();
        assertThat(missionModelRecords).isEmpty();
    }

    @Test
    void providerFailureMessagesRemainOutOfFrameMetadataAndAppearInDeliberateErrorDiagnostics() {
        String endpointSentinel = "http://127.0.0.1:1/SENTINEL-BASE";
        RuntimeException providerFailure = new IllegalStateException(
                "I/O error on POST request for \"" + endpointSentinel + "/v1/chat/completions\"");
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);

        LoomspanSession planningSession = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "planning-provider-failure", "test.entry", 3);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);

        assertThatThrownBy(() -> planningService.initializePlan(
                planningSession,
                "hello",
                null,
                rootDefinition(),
                new SequencePlanningChatClient(providerFailure),
                List.of()))
                .isInstanceOf(RuntimeException.class);

        assertSafeFailureRecords(readRecords(planningSession), endpointSentinel, "Planning model invocation failed");

        LoomspanSession missionSession = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                "mission-provider-failure", "test.entry", 3);
        try (var executor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService),
                    stateService,
                    Duration.ofSeconds(5),
                    executor);

            assertThatThrownBy(() -> engine.executeMission(
                    missionSession,
                    rootDefinition(),
                    "hello",
                    null,
                    new SequencePlanningChatClient(providerFailure),
                    List.of(),
                    false,
                    null))
                    .isInstanceOf(RuntimeException.class);
        }

        assertSafeFailureRecords(readRecords(missionSession), endpointSentinel, "Model invocation failed");
    }

    private static void assertSafeFailureRecords(List<TraceRecord> records, String sentinel, String safeMessage) {
        assertThat(records)
                .filteredOn(record -> record.recordType() == TraceRecordType.FRAME_CLOSED
                        && "failed".equals(record.metadata().get("status")))
                .isNotEmpty()
                .allSatisfy(record -> {
                    assertThat(record.metadata()).containsEntry("exceptionType", IllegalStateException.class.getName());
                    assertThat(record.metadata()).containsEntry("message", safeMessage);
                    assertThat(record.metadata().toString()).doesNotContain(sentinel);
                });
        assertThat(records)
                .filteredOn(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                .singleElement()
                .satisfies(record -> assertThat(record.data().toString())
                        .contains(sentinel, "JAVA_STACK_TRACE", "captureLimitBytes"));
    }

    @Test
    void planCreationIsOwnedByPlanningFrameNotNestedModelFrame() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("planning-owner-trace", "test.entry", 3);

        planningService.initializePlan(
                session,
                "hello",
                null,
                rootDefinition(),
                new SimpleChatClient(plan("plan-1"), "done"),
                List.<BoundCapability>of());

        TraceRecord planCreated = readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.PLAN_CREATED)
                .findFirst()
                .orElseThrow();

        assertThat(planCreated.frameType()).isEqualTo(TraceFrameType.PLANNING);
        assertThat(planCreated.route()).isEqualTo("rootVisibleSkill#planning");
    }

    @Test
    void planningQualityEventsStayUnderThePlanningFrame() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("planning-quality-trace", "test.entry", 3);

        planningService.initializePlan(
                session,
                "check invoice duplicates",
                null,
                duplicateInvoiceDefinition(),
                new SequencePlanningChatClient(weakPlanJson(), correctedPlanJson()),
                List.of(tool("invoiceParser", "Extract invoice fields from source documents"),
                        tool("expenseLookup", "Look up related expenses for comparison")));

        List<TraceRecord> records = readRecords(session);
        assertThat(records).anyMatch(record -> record.recordType() == TraceRecordType.PLAN_VALIDATION_FAILED
                && record.frameType() == TraceFrameType.PLANNING
                && record.metadata().containsKey("severity")
                && record.metadata().containsKey("issueCodes")
                && record.metadata().containsKey("retryCount"));
        assertThat(records).anyMatch(record -> record.recordType() == TraceRecordType.PLAN_RETRY_REQUESTED
                && record.frameType() == TraceFrameType.PLANNING);
    }

    @Test
    void exhaustedPlanQualityRetriesDegradeToPlanningWarningUnderPlanningFrame() {
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        DefaultPlanningService planningService = new DefaultPlanningService(new DefaultPlanTaskLinker(), stateService);
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("planning-quality-retry-cap-trace", "test.entry", 3);

        planningService.initializePlan(
                session,
                "check invoice duplicates",
                null,
                duplicateInvoiceDefinition(),
                new SequencePlanningChatClient(weakPlanJson(), weakPlanJson()),
                List.of(tool("invoiceParser", "Extract invoice fields from source documents"),
                        tool("expenseLookup", "Look up related expenses for comparison")));

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
                    assertThat(record.metadata()).containsKey("issueCodes");
                });
    }

    private static void assertEquivalentEnvelope(TraceRecord planningRecord, TraceRecord missionRecord) {
        assertThat(planningRecord.metadata().keySet()).containsExactlyElementsOf(missionRecord.metadata().keySet());
        assertThat(planningRecord.metadata()).containsEntry("frameworkModel", "gpt-5");
        assertThat(missionRecord.metadata()).containsEntry("frameworkModel", "gpt-5");
        assertThat(planningRecord.metadata()).containsEntry("connection", "test-connection");
        assertThat(missionRecord.metadata()).containsEntry("connection", "test-connection");
        assertThat(planningRecord.metadata()).containsEntry("driver", AiDriver.OPENAI.name());
        assertThat(missionRecord.metadata()).containsEntry("driver", AiDriver.OPENAI.name());
        assertThat(planningRecord.metadata()).containsEntry("providerModel", "openai/gpt-5");
        assertThat(missionRecord.metadata()).containsEntry("providerModel", "openai/gpt-5");
        assertThat(planningRecord.metadata()).containsEntry("skillName", "rootVisibleSkill");
        assertThat(missionRecord.metadata()).containsEntry("skillName", "rootVisibleSkill");
    }

    private static ExecutionPlan plan(String planId) {
        return new ExecutionPlan(
                planId,
                "rootVisibleSkill",
                Instant.parse("2026-03-15T12:00:00Z"),
                List.of());
    }

    private static YamlSkillDefinition rootDefinition() {
        return definition("rootVisibleSkill");
    }

    private static YamlSkillDefinition duplicateInvoiceDefinition() {
        return definition("duplicateInvoiceChecker");
    }

    private static YamlSkillDefinition definition(String name) {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName(name);
        manifest.setDescription(name);
        manifest.setModel("gpt-5");
        return new YamlSkillDefinition(
                new org.springframework.core.io.ByteArrayResource(new byte[0]),
                manifest,
                EXECUTION_CONFIGURATION);
    }

    private static List<TraceRecord> modelRecords(LoomspanSession session) {
        return readRecords(session).stream()
                .filter(record -> record.recordType() == TraceRecordType.MODEL_REQUEST_PREPARED
                        || record.recordType() == TraceRecordType.MODEL_REQUEST_SENT
                        || record.recordType() == TraceRecordType.MODEL_RESPONSE_RECEIVED)
                .toList();
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static BoundCapability tool(String name, String description) {
        return com.lokiscale.loomspan.testkit.TestBoundCapabilities.describedCapability(name, description);
    }

    private static String weakPlanJson() {
        return """
                {
                  "planId": "plan-weak",
                  "capabilityName": "duplicateInvoiceChecker",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {"taskId": "t-1", "title": "Parse invoice", "status": "PENDING", "capabilityName": "invoiceParser", "intent": "Extract invoice fields", "dependsOn": [], "expectedOutputs": ["parsed"], "autoCompletable": false, "note": ""},
                    {"taskId": "t-2", "title": "Check duplicates", "status": "PENDING", "capabilityName": "invoiceParser", "intent": "Check for matching expenses", "dependsOn": ["t-1"], "expectedOutputs": ["matches"], "autoCompletable": false, "note": ""},
                    {"taskId": "t-3", "title": "Final report", "status": "PENDING", "capabilityName": "invoiceParser", "intent": "Summarize duplicate findings", "dependsOn": ["t-2"], "expectedOutputs": ["report"], "autoCompletable": false, "note": ""}
                  ]
                }
                """;
    }

    private static String correctedPlanJson() {
        return """
                {
                  "planId": "plan-corrected",
                  "capabilityName": "duplicateInvoiceChecker",
                  "createdAt": "2026-03-15T12:00:00Z",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {"taskId": "t-1", "title": "Parse invoice", "status": "PENDING", "capabilityName": "invoiceParser", "intent": "Extract invoice fields", "dependsOn": [], "expectedOutputs": ["parsed"], "autoCompletable": false, "note": ""},
                    {"taskId": "t-2", "title": "Look up matches", "status": "PENDING", "capabilityName": "expenseLookup", "intent": "Find matching expenses", "dependsOn": ["t-1"], "expectedOutputs": ["matches"], "autoCompletable": false, "note": ""},
                    {"taskId": "t-3", "title": "Compare evidence", "status": "PENDING", "capabilityName": "expenseLookup", "intent": "Compare invoice and expenses", "dependsOn": ["t-2"], "expectedOutputs": ["decision"], "autoCompletable": false, "note": ""}
                  ]
                }
                """;
    }

    private static final class SequencePlanningChatClient implements com.lokiscale.loomspan.internal.model.ModelInteraction {

        private final Deque<String> responses = new ArrayDeque<>();
        private final RuntimeException failure;

        private SequencePlanningChatClient(String... responses) {
            this.failure = null;
            this.responses.addAll(List.of(responses));
        }

        private SequencePlanningChatClient(RuntimeException failure) {
            this.failure = failure;
        }

        @Override
        public com.lokiscale.loomspan.internal.model.ModelInteractionResult call(
                com.lokiscale.loomspan.internal.model.ModelInteractionRequest request) {
            if (failure != null) {
                throw failure;
            }
            String next = responses.pollFirst();
            if (next == null) {
                throw new IllegalStateException("No more queued chat responses");
            }
            return com.lokiscale.loomspan.internal.model.ModelInteractionResult.content(next);
        }
    }
}