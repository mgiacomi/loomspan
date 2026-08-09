package com.lokiscale.loomspan.internal.skill;

import com.lokiscale.loomspan.autoconfigure.AiDriver;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException;
import com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.SimpleChatClient;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import org.junit.jupiter.api.Test;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.core.io.ByteArrayResource;

import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class MissionExecutionEngineTest {

    private static final Clock FIXED_CLOCK = Clock.fixed(Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC);
    private static final EffectiveSkillExecutionConfiguration EXECUTION_CONFIGURATION =
            new EffectiveSkillExecutionConfiguration("gpt-5", "test-connection", AiDriver.OPENAI, "openai/gpt-5", "medium");

    @Test
    void executesPlanningEnabledMissionLoop() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
            ExecutionPlan plan = new ExecutionPlan(
                    "plan-1",
                    "rootVisibleSkill",
                    Instant.parse("2026-03-15T12:00:00Z"),
                    List.of(
                            new PlanTask("task-1", "Use tool", PlanTaskStatus.PENDING, "allowedVisibleSkill", "Use tool", List.of(), List.of(), false, null),
                            new PlanTask("task-2", "Blocked", PlanTaskStatus.BLOCKED, null)));
            MissionChatClient chatClient = new MissionChatClient("mission complete");
            ToolCallback callback = mock(ToolCallback.class);

            when(planningService.initializePlan(eq(session), eq("hello"), eq(null), any(YamlSkillDefinition.class), eq(chatClient), any()))
                    .thenAnswer(invocation -> {
                        stateService.storePlan(session, plan);
                        return java.util.Optional.of(plan);
                    });

            String response = engine.executeMission(session, definition(), "hello", null, chatClient, List.of(callback), true, null);

            assertThat(response).isEqualTo("mission complete");
            assertThat(chatClient.getSystemMessagesSeen().getFirst()).contains("plan-1", "Ready tasks", "Blocked tasks");
        }
    }

    @Test
    void skipsPlanningForPlanningDisabledMission() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-1", "test.entry", 2);
            MissionChatClient chatClient = new MissionChatClient("mission complete");

            String response = engine.executeMission(session, definition(), "hello", null, chatClient, List.of(), false, null);

            assertThat(response).isEqualTo("mission complete");
            assertThat(chatClient.getSystemMessagesSeen()).containsExactly("Execute the mission using only the visible YAML tools when needed.");
            verify(planningService, never()).initializePlan(eq(session), eq("hello"), eq(null), any(YamlSkillDefinition.class), eq(chatClient), any());
        }
    }

    @Test
    void prependsSkillPromptToSingleShotExecutionPrompt() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-prompt", "test.entry", 2);
            MissionChatClient chatClient = new MissionChatClient("mission complete");

            String response = engine.executeMission(session, definitionWithPrompt(), "hello", null, chatClient, List.of(), false, null);

            assertThat(response).isEqualTo("mission complete");
            assertThat(chatClient.getSystemMessagesSeen()).hasSize(1);
            assertThat(chatClient.getSystemMessagesSeen().getFirst())
                    .startsWith("Act as a careful parser.")
                    .contains("Execute the mission using only the visible YAML tools when needed.");

            assertThat(readRecords(session)).noneMatch(record ->
                    record.recordType() == TraceRecordType.MODEL_REQUEST_SENT);
        }
    }

    @Test
    void recordsFullMissionRequestPayloadWhenRequestIsSent() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-request-trace", "test.entry", 2);
            ToolCallback callback = mock(ToolCallback.class);

            String response = engine.executeMission(
                    session,
                    definition(),
                    "hello",
                    null,
                    new MissionChatClient("mission complete"),
                    List.of(callback),
                    false,
                    null);

            assertThat(response).isEqualTo("mission complete");
            assertThat(readRecords(session)).noneMatch(record ->
                    record.recordType() == TraceRecordType.MODEL_REQUEST_SENT);
        }
    }

    @Test
    void sendsDeclaredImageAttachmentAsMediaInsteadOfTextOnlyUserMessage() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-attachment", "test.entry", 2);
            MissionChatClient chatClient = new MissionChatClient("mission complete");

            String response = engine.executeMission(
                    session,
                    attachmentDefinition(),
                    "Extract ticket",
                    Map.of("image", imageResource("ticket.jpg", "SECRET_IMAGE_BYTES")),
                    chatClient,
                    List.of(),
                    false,
                    null);

            assertThat(response).isEqualTo("mission complete");
            assertThat(chatClient.getUserMediaSeen()).hasSize(1);
            assertThat(chatClient.getUserMediaSeen().getFirst().mimeType().toString()).isEqualTo("image/jpeg");
            assertThat(chatClient.getUserMessagesSeen()).hasSize(1);
            assertThat(chatClient.getUserMessagesSeen().getFirst())
                    .contains("\"attachment\" : true", "\"contentType\" : \"image/jpeg\"")
                    .doesNotContain("SECRET_IMAGE_BYTES")
                    .doesNotContain("ByteArrayResource");
        }
    }

    @Test
    void planningReceivesDescriptorsAndMissionTraceRedactsAttachmentBytes() {
        PlanningService planningService = mock(PlanningService.class);
        AtomicReference<Map<String, Object>> planningInput = new AtomicReference<>();
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-attachment-trace", "test.entry", 2);
            MissionChatClient chatClient = new MissionChatClient("mission complete");
            ExecutionPlan plan = new ExecutionPlan(
                    "plan-attachment",
                    "rootVisibleSkill",
                    Instant.parse("2026-03-15T12:00:00Z"),
                    List.of(new PlanTask("task-1", "Inspect ticket", PlanTaskStatus.PENDING, null)));
            when(planningService.initializePlan(eq(session), eq("Extract ticket"), any(), any(YamlSkillDefinition.class), eq(chatClient), any()))
                    .thenAnswer(invocation -> {
                        @SuppressWarnings("unchecked")
                        Map<String, Object> input = (Map<String, Object>) invocation.getArgument(2);
                        planningInput.set(input);
                        stateService.storePlan(session, plan);
                        return java.util.Optional.of(plan);
                    });

            engine.executeMission(
                    session,
                    attachmentDefinition(),
                    "Extract ticket",
                    Map.of("image", imageResource("ticket.jpg", "SECRET_IMAGE_BYTES")),
                    chatClient,
                    List.of(),
                    true,
                    null);

            assertThat(planningInput.get().get("image")).isInstanceOf(Map.class);
            assertThat(String.valueOf(planningInput.get())).contains("attachment=true").doesNotContain("SECRET_IMAGE_BYTES");
            assertThat(chatClient.getUserMediaSeen()).hasSize(1);

            List<String> tracePayloads = readRecords(session).stream()
                    .map(String::valueOf)
                    .toList();
            assertThat(tracePayloads).isNotEmpty();
            assertThat(tracePayloads).allSatisfy(payload -> assertThat(payload)
                    .doesNotContain("SECRET_IMAGE_BYTES")
                    .doesNotContain("base64")
                    .doesNotContain("ByteArrayResource"));
        }
    }

    @Test
    void wrapsProviderFailureWithAttachmentContext() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);

            assertThatThrownBy(() -> engine.executeMission(
                    com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-provider-failure", "test.entry", 2),
                    attachmentDefinition(),
                    "Extract ticket",
                    Map.of("image", imageResource("ticket.jpg", "image bytes")),
                    new FailingMissionChatClient(),
                    List.of(),
                    false,
                    null))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("rootVisibleSkill")
                    .hasMessageContaining("openai/gpt-5")
                    .hasMessageContaining("IMAGE/image/jpeg")
                    .hasMessageContaining("supports the declared attachment media");
        }
    }

    @Test
    void timesOutBlockingMissionExecution() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        AtomicBoolean interrupted = new AtomicBoolean(false);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofMillis(25),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-timeout", "test.entry", 2);
            BlockingMissionChatClient chatClient = new BlockingMissionChatClient(interrupted);

            assertThatThrownBy(() -> engine.executeMission(
                    session,
                    definition(),
                    "hello",
                    null,
                    chatClient,
                    List.of(),
                    false,
                    null))
                    .isInstanceOf(LoomspanMissionTimeoutException.class)
                    .hasMessageContaining("session-timeout")
                    .hasMessageContaining("rootVisibleSkill")
                    .hasMessageContaining("PT0.025S");
            awaitInterrupted(interrupted);
            awaitFramesCleared(session);

            List<TraceRecord> records = readRecords(session);
            TraceRecord error = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                    .findFirst()
                    .orElseThrow();
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED))
                    .hasSize(1);
            TraceRecord frameClosed = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                    .findFirst()
                    .orElseThrow();
            assertThat(records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                    .count()).isEqualTo(1);
            assertThat(frameClosed.metadata()).containsEntry("status", "aborted");
            assertThat(error.frameId()).isEqualTo(frameClosed.frameId());
            assertThat(error.frameType()).isEqualTo(TraceFrameType.MODEL_CALL);
            assertThat(records.indexOf(error)).isLessThan(records.indexOf(frameClosed));
        }
    }

    @Test
    void timesOutWhilePlanningEnabledMissionInitializesPlan() {
        AtomicBoolean interrupted = new AtomicBoolean(false);
        PlanningService planningService = new BlockingPlanningService(interrupted);
        ExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);

        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofMillis(25),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-timeout", "test.entry", 2);
            MissionChatClient chatClient = new MissionChatClient("mission complete");

            assertThatThrownBy(() -> engine.executeMission(
                    session,
                    definition(),
                    "hello",
                    null,
                    chatClient,
                    List.of(),
                    true,
                    null))
                    .isInstanceOf(LoomspanMissionTimeoutException.class)
                    .hasMessageContaining("session-timeout")
                    .hasMessageContaining("rootVisibleSkill")
                    .hasMessageContaining("PT0.025S");
            awaitInterrupted(interrupted);
            assertThat(session.getExecutionPlan()).isEmpty();
        }
    }

    @Test
    void recordsFailedModelFrameStatusWhenMissionCallThrows() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("session-failure", "test.entry", 2);

            assertThatThrownBy(() -> engine.executeMission(
                    session,
                    definition(),
                    "hello",
                    null,
                    new FailingMissionChatClient(),
                    List.of(),
                    false,
                    null))
                    .isInstanceOf(IllegalStateException.class)
                    .hasMessageContaining("boom");

            List<TraceRecord> records = readRecords(session);
            TraceRecord frameClosed = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                    .filter(record -> "failed".equals(record.metadata().get("status")))
                    .findFirst()
                    .orElseThrow();
            TraceRecord error = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                    .findFirst()
                    .orElseThrow();

            assertThat(frameClosed.metadata()).containsEntry("status", "failed");
            assertThat(frameClosed.metadata()).containsEntry("exceptionType", IllegalStateException.class.getName());
            assertThat(error.frameId()).isEqualTo(frameClosed.frameId());
            assertThat(error.frameType()).isEqualTo(TraceFrameType.MODEL_CALL);
            assertThat(error.data().path("diagnostics").get(0).path("kind").asText()).isEqualTo("JAVA_STACK_TRACE");
            assertThat(records.indexOf(error)).isLessThan(records.indexOf(frameClosed));
        }
    }

    @Test
    void recordsAndRethrowsErrorAtModelFrame() {
        PlanningService planningService = mock(PlanningService.class);
        DefaultExecutionStateService stateService = new DefaultExecutionStateService(FIXED_CLOCK);
        AssertionError failure = new AssertionError("fatal model failure");
        try (ExecutorService missionExecutor = Executors.newVirtualThreadPerTaskExecutor()) {
            DefaultMissionExecutionEngine engine = new DefaultMissionExecutionEngine(
                    planningService,
                    stateService,
                    Duration.ofSeconds(5),
                    missionExecutor);
            LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId(
                    "session-error", "test.entry", 2);

            assertThatThrownBy(() -> engine.executeMission(
                    session,
                    definition(),
                    "hello",
                    null,
                    new FailingMissionChatClient(failure),
                    List.of(),
                    false,
                    null))
                    .isSameAs(failure);

            List<TraceRecord> records = readRecords(session);
            TraceRecord error = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.ERROR_RECORDED)
                    .findFirst()
                    .orElseThrow();
            TraceRecord frameClosed = records.stream()
                    .filter(record -> record.recordType() == TraceRecordType.FRAME_CLOSED)
                    .findFirst()
                    .orElseThrow();

            assertThat(error.frameId()).isEqualTo(frameClosed.frameId());
            assertThat(error.frameType()).isEqualTo(TraceFrameType.MODEL_CALL);
            assertThat(frameClosed.metadata()).containsEntry("status", "failed");
            assertThat(records.indexOf(error)).isLessThan(records.indexOf(frameClosed));
        }
    }

    private void awaitInterrupted(AtomicBoolean interrupted) {
        long deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(1);
        while (!interrupted.get() && System.nanoTime() < deadlineNanos) {
            Thread.onSpinWait();
        }
        assertThat(interrupted.get()).isTrue();
    }

    private void awaitFramesCleared(LoomspanSession session) {
        long deadlineNanos = System.nanoTime() + TimeUnit.SECONDS.toNanos(1);
        while (!session.getFramesSnapshot().isEmpty() && System.nanoTime() < deadlineNanos) {
            Thread.onSpinWait();
        }
        assertThat(session.getFramesSnapshot()).isEmpty();
    }

    private static List<TraceRecord> readRecords(LoomspanSession session) {
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        return records;
    }

    private static YamlSkillDefinition definition() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        return new YamlSkillDefinition(new org.springframework.core.io.ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition definitionWithPrompt() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
        manifest.setPrompt("Act as a careful parser.");
        return new YamlSkillDefinition(new org.springframework.core.io.ByteArrayResource(new byte[0]), manifest, EXECUTION_CONFIGURATION);
    }

    private static YamlSkillDefinition attachmentDefinition() {
        YamlSkillManifest manifest = new YamlSkillManifest();
        manifest.setName("rootVisibleSkill");
        manifest.setDescription("rootVisibleSkill");
        manifest.setModel("gpt-5");
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
        byte[] marker = content.getBytes(java.nio.charset.StandardCharsets.UTF_8);
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

    private static final class MissionChatClient extends SimpleChatClient {

        private MissionChatClient(String content) {
            super(null, content);
        }
    }

    private static final class BlockingMissionChatClient extends SimpleChatClient {

        private final AtomicBoolean interrupted;

        private BlockingMissionChatClient(AtomicBoolean interrupted) {
            super(null, "unused");
            this.interrupted = interrupted;
        }

        @Override
        public ChatClientRequestSpec prompt() {
            return new BlockingRequestSpec();
        }

        private final class BlockingRequestSpec implements ChatClientRequestSpec {

            @Override
            public Builder mutate() {
                throw new UnsupportedOperationException();
            }

            @Override
            public ChatClientRequestSpec advisors(java.util.function.Consumer<AdvisorSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(org.springframework.ai.chat.client.advisor.api.Advisor... advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(List<org.springframework.ai.chat.client.advisor.api.Advisor> advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(org.springframework.ai.chat.messages.Message... messages) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(List<org.springframework.ai.chat.messages.Message> messages) {
                return this;
            }

            @Override
            public <T extends org.springframework.ai.chat.prompt.ChatOptions> ChatClientRequestSpec options(T options) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolNames(String... toolNames) {
                return this;
            }

            @Override
            public ChatClientRequestSpec tools(Object... tools) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(ToolCallback... toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(List<ToolCallback> toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(org.springframework.ai.tool.ToolCallbackProvider... providers) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolContext(java.util.Map<String, Object> toolContext) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(String text) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(java.util.function.Consumer<PromptSystemSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(String text) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(java.util.function.Consumer<PromptUserSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec templateRenderer(org.springframework.ai.template.TemplateRenderer renderer) {
                return this;
            }

            @Override
            public CallResponseSpec call() {
                return new BlockingResponseSpec();
            }

            @Override
            public StreamResponseSpec stream() {
                throw new UnsupportedOperationException();
            }
        }

        private final class BlockingResponseSpec implements CallResponseSpec {

            @Override
            public <T> T entity(org.springframework.core.ParameterizedTypeReference<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> T entity(org.springframework.ai.converter.StructuredOutputConverter<T> converter) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> T entity(Class<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public org.springframework.ai.chat.client.ChatClientResponse chatClientResponse() {
                throw new UnsupportedOperationException();
            }

            @Override
            public org.springframework.ai.chat.model.ChatResponse chatResponse() {
                throw new UnsupportedOperationException();
            }

            @Override
            public String content() {
                try {
                    new CountDownLatch(1).await();
                    throw new AssertionError("Latch await returned unexpectedly");
                }
                catch (InterruptedException ex) {
                    interrupted.set(true);
                    throw new IllegalStateException("provider interrupted", ex);
                }
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<org.springframework.ai.chat.model.ChatResponse, T> responseEntity(Class<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<org.springframework.ai.chat.model.ChatResponse, T> responseEntity(
                    org.springframework.core.ParameterizedTypeReference<T> type) {
                throw new UnsupportedOperationException();
            }

            @Override
            public <T> org.springframework.ai.chat.client.ResponseEntity<org.springframework.ai.chat.model.ChatResponse, T> responseEntity(
                    org.springframework.ai.converter.StructuredOutputConverter<T> converter) {
                throw new UnsupportedOperationException();
            }
        }
    }

    private static final class FailingMissionChatClient extends SimpleChatClient {

        private final Throwable failure;

        private FailingMissionChatClient() {
            this(new IllegalStateException("boom"));
        }

        private FailingMissionChatClient(Throwable failure) {
            super(null, "unused");
            this.failure = failure;
        }

        @Override
        public ChatClientRequestSpec prompt() {
            return new FailingRequestSpec(failure);
        }

        private static final class FailingRequestSpec implements ChatClientRequestSpec {

            private final Throwable failure;

            private FailingRequestSpec(Throwable failure) {
                this.failure = failure;
            }

            @Override
            public Builder mutate() {
                throw new UnsupportedOperationException();
            }

            @Override
            public ChatClientRequestSpec advisors(java.util.function.Consumer<AdvisorSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(org.springframework.ai.chat.client.advisor.api.Advisor... advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec advisors(List<org.springframework.ai.chat.client.advisor.api.Advisor> advisors) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(org.springframework.ai.chat.messages.Message... messages) {
                return this;
            }

            @Override
            public ChatClientRequestSpec messages(List<org.springframework.ai.chat.messages.Message> messages) {
                return this;
            }

            @Override
            public <T extends org.springframework.ai.chat.prompt.ChatOptions> ChatClientRequestSpec options(T options) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolNames(String... toolNames) {
                return this;
            }

            @Override
            public ChatClientRequestSpec tools(Object... tools) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(ToolCallback... toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(List<ToolCallback> toolCallbacks) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolCallbacks(org.springframework.ai.tool.ToolCallbackProvider... providers) {
                return this;
            }

            @Override
            public ChatClientRequestSpec toolContext(java.util.Map<String, Object> toolContext) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(String text) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec system(java.util.function.Consumer<PromptSystemSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(String text) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource, java.nio.charset.Charset charset) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(org.springframework.core.io.Resource resource) {
                return this;
            }

            @Override
            public ChatClientRequestSpec user(java.util.function.Consumer<PromptUserSpec> consumer) {
                return this;
            }

            @Override
            public ChatClientRequestSpec templateRenderer(org.springframework.ai.template.TemplateRenderer renderer) {
                return this;
            }

            @Override
            public CallResponseSpec call() {
                if (failure instanceof RuntimeException runtimeException) {
                    throw runtimeException;
                }
                throw (Error) failure;
            }

            @Override
            public StreamResponseSpec stream() {
                throw new UnsupportedOperationException();
            }
        }
    }

    private static final class BlockingPlanningService implements PlanningService {

        private final AtomicBoolean interrupted;

        private BlockingPlanningService(AtomicBoolean interrupted) {
            this.interrupted = interrupted;
        }

        @Override
        public java.util.Optional<ExecutionPlan> initializePlan(LoomspanSession session,
                                                                String objective,
                                                                java.util.Map<String, Object> missionInput,
                                                                YamlSkillDefinition definition,
                                                                org.springframework.ai.chat.client.ChatClient chatClient,
                                                                List<ToolCallback> visibleTools) {
            try {
                new CountDownLatch(1).await();
                throw new AssertionError("Latch await returned unexpectedly");
            }
            catch (InterruptedException ex) {
                interrupted.set(true);
                Thread.currentThread().interrupt();
                throw new IllegalStateException("Planning interrupted", ex);
            }
        }

        @Override
        public java.util.Optional<ExecutionPlan> markToolStarted(LoomspanSession session,
                                                                 com.lokiscale.loomspan.internal.core.CapabilityMetadata capability,
                                                                 java.util.Map<String, Object> arguments) {
            throw new UnsupportedOperationException();
        }

        @Override
        public java.util.Optional<ExecutionPlan> markTaskStarted(LoomspanSession session,
                                                                 String taskId,
                                                                 String capabilityName,
                                                                 java.util.Map<String, Object> arguments) {
            throw new UnsupportedOperationException();
        }

        @Override
        public java.util.Optional<ExecutionPlan> markToolCompleted(LoomspanSession session,
                                                                   String taskId,
                                                                   String capabilityName,
                                                                   Object result) {
            throw new UnsupportedOperationException();
        }

        @Override
        public java.util.Optional<ExecutionPlan> markToolFailed(LoomspanSession session,
                                                                String taskId,
                                                                String capabilityName,
                                                                RuntimeException ex) {
            throw new UnsupportedOperationException();
        }
    }
}
