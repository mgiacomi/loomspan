package com.lokiscale.loomspan.internal.runtime.step;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.LoomspanStackOverflowException;
import com.lokiscale.loomspan.internal.core.CapabilityRegistry;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.SessionContextRunner;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceFailureMetadata;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.linter.LinterOutcome;
import com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationIssue;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationResult;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidator;
import com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException;
import com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.attachment.DefaultMissionInputMaterializer;
import com.lokiscale.loomspan.internal.runtime.attachment.MissionInputMaterializer;
import com.lokiscale.loomspan.internal.runtime.attachment.RenderedMissionInput;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceBackedOutputValidator;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageResult;
import com.lokiscale.loomspan.internal.runtime.planning.PlanningService;
import com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposer;
import com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposition;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.vfs.DefaultRefResolver;
import com.lokiscale.loomspan.internal.vfs.SessionLocalVirtualFileSystem;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.time.Duration;
import java.nio.file.Paths;
import java.util.LinkedHashMap;
import java.util.ArrayDeque;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.concurrent.Callable;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicReference;
import java.util.function.Function;
import java.util.stream.Collectors;

/**
 * Plan-step execution engine that replaces the single-shot mission model call with a deterministic loop.
 */
public class StepLoopMissionExecutionEngine implements MissionExecutionEngine
{
    private static final Logger log = LoggerFactory.getLogger(StepLoopMissionExecutionEngine.class);

    private static final int DEFAULT_MAX_STEPS = 10;
    private static final int MAX_INVALID_ACTION_RETRIES = 1;
    private static final int MAX_EXECUTION_SUMMARY_LINES = 5;

    private enum CleanupOwner
    {
        NONE,
        WORKER,
        CALLER
    }

    private final PlanningService planningService;
    private final ExecutionStateService executionStateService;
    @SuppressWarnings("unused")
    private final CapabilityRegistry capabilityRegistry;
    private @Nullable YamlSkillCatalog yamlSkillCatalog;
    private final Duration missionTimeout;
    private final ExecutorService missionExecutor;
    private final SessionUsageService sessionUsageService;
    private final ObjectMapper objectMapper;
    private final OutputSchemaValidator outputSchemaValidator;
    private final EvidenceBackedOutputValidator evidenceBackedOutputValidator;
    private final int defaultMaxSteps;
    private final MissionInputMaterializer missionInputMaterializer;

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            YamlSkillCatalog ignoredYamlSkillCatalog,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, DEFAULT_MAX_STEPS);
        this.yamlSkillCatalog = Objects.requireNonNull(ignoredYamlSkillCatalog, "yamlSkillCatalog must not be null");
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            YamlSkillCatalog yamlSkillCatalog,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            MissionInputMaterializer missionInputMaterializer,
            ObjectMapper schemaMapper)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, DEFAULT_MAX_STEPS, missionInputMaterializer, schemaMapper);
        this.yamlSkillCatalog = Objects.requireNonNull(yamlSkillCatalog, "yamlSkillCatalog must not be null");
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            YamlSkillCatalog ignoredYamlSkillCatalog,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            int defaultMaxSteps)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, defaultMaxSteps, defaultMaterializer());
        this.yamlSkillCatalog = Objects.requireNonNull(ignoredYamlSkillCatalog, "yamlSkillCatalog must not be null");
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, DEFAULT_MAX_STEPS);
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            int defaultMaxSteps)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, defaultMaxSteps, defaultMaterializer());
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            YamlSkillCatalog ignoredYamlSkillCatalog,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            MissionInputMaterializer missionInputMaterializer)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, DEFAULT_MAX_STEPS, missionInputMaterializer);
        this.yamlSkillCatalog = Objects.requireNonNull(ignoredYamlSkillCatalog, "yamlSkillCatalog must not be null");
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            int defaultMaxSteps,
            MissionInputMaterializer missionInputMaterializer)
    {
        this(planningService, executionStateService, capabilityRegistry, missionTimeout, missionExecutor,
                sessionUsageService, defaultMaxSteps, missionInputMaterializer,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().schemaTree());
    }

    public StepLoopMissionExecutionEngine(PlanningService planningService,
            ExecutionStateService executionStateService,
            CapabilityRegistry capabilityRegistry,
            Duration missionTimeout,
            ExecutorService missionExecutor,
            SessionUsageService sessionUsageService,
            int defaultMaxSteps,
            MissionInputMaterializer missionInputMaterializer,
            ObjectMapper schemaMapper)
    {
        this.planningService = Objects.requireNonNull(planningService, "planningService must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.capabilityRegistry = Objects.requireNonNull(capabilityRegistry, "capabilityRegistry must not be null");
        this.yamlSkillCatalog = null;
        this.missionTimeout = Objects.requireNonNull(missionTimeout, "missionTimeout must not be null");
        this.missionExecutor = Objects.requireNonNull(missionExecutor, "missionExecutor must not be null");
        this.sessionUsageService = Objects.requireNonNull(sessionUsageService, "sessionUsageService must not be null");
        this.objectMapper = Objects.requireNonNull(schemaMapper, "schemaMapper must not be null");
        this.outputSchemaValidator = new OutputSchemaValidator(schemaMapper);
        this.evidenceBackedOutputValidator = new EvidenceBackedOutputValidator(schemaMapper,
                new com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageValidator());
        this.defaultMaxSteps = defaultMaxSteps;
        this.missionInputMaterializer = Objects.requireNonNull(missionInputMaterializer, "missionInputMaterializer must not be null");
    }

    @Override
    public String executeMission(LoomspanSession session,
            YamlSkillDefinition definition,
            String objective,
            @Nullable Map<String, Object> missionInput,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools,
            boolean planningEnabled,
            @Nullable Authentication authentication)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(definition, "definition must not be null");
        Objects.requireNonNull(objective, "objective must not be null");
        Objects.requireNonNull(modelInteraction, "modelInteraction must not be null");
        Objects.requireNonNull(visibleTools, "visibleTools must not be null");
        String skillName = definition.manifest().getName();
        EffectiveSkillExecutionConfiguration executionConfiguration = definition.requireExecutionConfiguration();

        int baselineFrameDepth = session.getFramesSnapshot().size();
        AtomicReference<CleanupOwner> cleanupOwner = new AtomicReference<>(CleanupOwner.NONE);
        CountDownLatch cleanupComplete = new CountDownLatch(1);

        Callable<String> missionCall = () -> SessionContextRunner.callWithSession(session, () ->
        {
            try
            {
                sessionUsageService.recordMissionStart(session, skillName);
                RenderedMissionInput renderedInput = missionInputMaterializer.materialize(session, definition, objective, missionInput);

                if (planningEnabled)
                {
                    planningService.initializePlan(
                            session,
                            objective,
                            planningInput(missionInput, renderedInput),
                            definition,
                            modelInteraction,
                            visibleTools);
                }

                Optional<ExecutionPlan> planOpt = executionStateService.currentPlan(session);
                if (planOpt.isEmpty())
                {
                    throw new IllegalStateException(
                            "Step-loop execution requires a plan but none was created for skill '" + skillName + "'");
                }

                return executeStepLoop(session, definition, objective, renderedInput, executionConfiguration, modelInteraction, visibleTools,
                        cleanupOwner);
            }
            finally
            {
                try
                {
                    if (cleanupOwner.compareAndSet(CleanupOwner.NONE, CleanupOwner.WORKER))
                    {
                        unwindMissionFrames(session, baselineFrameDepth);
                    }
                }
                finally
                {
                    cleanupComplete.countDown();
                }
            }
        });

        Future<String> mission = missionExecutor.submit(missionCall);
        try
        {
            return mission.get(missionTimeout.toMillis(), TimeUnit.MILLISECONDS);
        }
        catch (TimeoutException ex)
        {
            boolean callerOwnsCleanup = cleanupOwner.compareAndSet(CleanupOwner.NONE, CleanupOwner.CALLER);
            LoomspanMissionTimeoutException failure = new LoomspanMissionTimeoutException(
                    session.getSessionId(), skillName, missionTimeout, ex);
            executionStateService.recordFailure(session, failure, Map.of("message", "Mission execution timed out"));
            mission.cancel(true);
            awaitMissionCleanup(session, mission, baselineFrameDepth, cleanupOwner, cleanupComplete, callerOwnsCleanup);
            throw failure;
        }
        catch (InterruptedException ex)
        {
            boolean callerOwnsCleanup = cleanupOwner.compareAndSet(CleanupOwner.NONE, CleanupOwner.CALLER);
            LoomspanMissionTimeoutException failure = new LoomspanMissionTimeoutException(
                    session.getSessionId(), skillName, missionTimeout, ex);
            executionStateService.recordFailure(session, failure, Map.of("message", "Mission execution interrupted"));
            mission.cancel(true);
            awaitMissionCleanup(session, mission, baselineFrameDepth, cleanupOwner, cleanupComplete, callerOwnsCleanup);
            Thread.currentThread().interrupt();
            throw failure;
        }
        catch (ExecutionException ex)
        {
            awaitMissionCleanup(session, mission, baselineFrameDepth, cleanupOwner, cleanupComplete, false);
            Throwable cause = ex.getCause();
            if (cause instanceof RuntimeException runtimeException)
            {
                throw unwrapMissionFailure(runtimeException);
            }
            if (cause instanceof Error error)
            {
                throw error;
            }
            throw new IllegalStateException("Mission execution failed for skill '" + skillName + "'", cause);
        }
    }

    private String executeStepLoop(LoomspanSession session,
            YamlSkillDefinition definition,
            String objective,
            RenderedMissionInput renderedInput,
            EffectiveSkillExecutionConfiguration executionConfiguration,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools,
            AtomicReference<CleanupOwner> cleanupOwner)
    {
        String skillName = definition.manifest().getName();
        int maxSteps = definition.maxSteps(defaultMaxSteps);
        if (maxSteps <= 0)
        {
            IllegalStateException failure = new IllegalStateException(
                        "Step-loop execution requires max_steps > 0 for skill '%s' but was %d."
                            .formatted(skillName, maxSteps));
            recordTerminalFailure(session, skillName, 0, failure,
                    "Step-loop execution rejected invalid max_steps value: " + maxSteps);
            throw failure;
        }

        validatePlanForStepLoop(session, skillName, executionStateService.currentPlan(session)
                .orElseThrow(() -> new IllegalStateException(
                        "Plan disappeared before step loop started for skill '" + skillName + "'")));

        Deque<String> executionSummary = new ArrayDeque<>();
        String lastToolResult = null;

        for (int stepNumber = 1; stepNumber <= maxSteps; stepNumber++)
        {
            if (Thread.currentThread().isInterrupted())
            {
                throw new LoomspanMissionTimeoutException(session.getSessionId(), skillName, missionTimeout,
                        new InterruptedException("Step loop interrupted"));
            }

            ExecutionPlan plan = executionStateService.currentPlan(session)
                    .orElseThrow(() -> new IllegalStateException(
                            "Plan disappeared during step loop for skill '" + skillName + "'"));

            List<PlanTask> readyTasks = plan.readyTasks();
            boolean allDone = plan.tasks().stream().allMatch(task -> task.status() == PlanTaskStatus.COMPLETED);
            boolean noneReady = readyTasks.isEmpty();

            if (allDone)
            {
                log.debug("All tasks completed at step {} for skill '{}', requesting final response", stepNumber, skillName);
            }

            if (noneReady && !allDone)
            {
                IllegalStateException failure = new IllegalStateException(
                        "Step-loop deadlock at step %d for skill '%s': no ready tasks remain while the plan is incomplete."
                                .formatted(stepNumber, skillName));
                recordTerminalFailure(session, skillName, stepNumber, failure,
                        "Plan deadlock: no ready tasks remain while the plan still has incomplete tasks.");
                throw failure;
            }

            StepResult stepResult = executeOneStep(
                    session,
                    skillName,
                    objective,
                    renderedInput,
                    executionConfiguration,
                    modelInteraction,
                    visibleTools,
                    plan,
                    stepNumber,
                    lastToolResult,
                    formatExecutionSummary(executionSummary),
                    definition,
                    true,
                    allDone,
                    cleanupOwner);

            if (stepResult.isFinalResponse())
            {
                return stepResult.finalResponse();
            }

            lastToolResult = stepResult.toolResult();
            if (stepResult.summaryLine() != null)
            {
                appendExecutionSummary(executionSummary, stepResult.summaryLine());
            }
        }

        IllegalStateException failure = new IllegalStateException("Step-loop exhausted %d steps for skill '%s' before the plan completed."
                .formatted(maxSteps, skillName));
        recordTerminalFailure(session, skillName, maxSteps, failure, "Step limit reached before the plan completed.");
        throw failure;
    }

    private StepResult executeOneStep(LoomspanSession session,
            String skillName,
            String objective,
            RenderedMissionInput renderedInput,
            EffectiveSkillExecutionConfiguration executionConfiguration,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools,
            ExecutionPlan plan,
            int stepNumber,
            @Nullable String lastToolResult,
            @Nullable String executionSummary,
            @Nullable YamlSkillDefinition skillDefinition,
            boolean strictCompletion,
            boolean finalResponseOnly,
            AtomicReference<CleanupOwner> cleanupOwner)
    {
        ExecutionFrame stepFrame = executionStateService.openFrame(
                session,
                TraceFrameType.STEP_EXECUTION,
                skillName + "#step-" + stepNumber,
                Map.of("stepNumber", stepNumber));

        executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_STARTED,
                Map.of("stepNumber", stepNumber, "readyTasks", plan.readyTasks().size()),
                Map.of("planStatus", plan.status().name()));

        String stepFrameStatus = "completed";
        Throwable stepFailure = null;
        StepAction lastAction = null;
        boolean forceVerboseToolArgumentGuidance = false;
        try
        {
            int invalidActionRetryCount = 0;
            int linterAttempt = 1;
            int outputSchemaAttempt = 1;
            int evidenceAttempt = 1;
            String invalidActionFeedback = null;
            while (true)
            {
                lastAction = null;
                String stepPrompt = StepPromptBuilder.buildStepPrompt(
                        plan,
                        objective,
                        renderedInput.traceSafeInput(),
                        stepNumber,
                        lastToolResult,
                        executionSummary,
                        visibleTools,
                        finalResponseOnly,
                        forceVerboseToolArgumentGuidance,
                        skillDefinition == null ? null : skillDefinition.outputSchema());
                SkillPromptComposition promptComposition = skillDefinition == null
                        ? new SkillPromptComposition(stepPrompt, false, null, "step_execution_prompt")
                        : SkillPromptComposer.composeStepExecutionPrompt(skillDefinition, stepPrompt);
                String stepUserMessage = StepPromptBuilder.buildStepUserMessage(plan, objective, renderedInput.traceSafeInput());
                String effectivePrompt = invalidActionFeedback == null || invalidActionFeedback.isBlank()
                        ? promptComposition.systemPrompt()
                        : promptComposition.systemPrompt() + "\n\nYOUR PREVIOUS ACTION WAS INVALID: "
                                + invalidActionFeedback + "\nPlease correct and try again.";

                String modelResponse = callModelForStep(
                        session, skillName, executionConfiguration, modelInteraction, effectivePrompt,
                        new RenderedMissionInput(stepUserMessage, renderedInput.attachments(), renderedInput.traceSafeInput()),
                        stepNumber,
                        promptComposition.traceMetadata(),
                        cleanupOwner);

                StepAction action = parseStepAction(modelResponse, finalResponseOnly);
                if (action == null)
                {
                    executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_ACTION_REJECTED,
                            Map.of("retry", invalidActionRetryCount, "reason", "Failed to parse model response as StepAction"),
                            Map.of("rawResponse", truncate(modelResponse, 500)));
                    if (invalidActionRetryCount >= MAX_INVALID_ACTION_RETRIES)
                    {
                        IllegalStateException failure = new IllegalStateException(
                                "Model failed to produce a valid step action after %d attempts at step %d for skill '%s'."
                                        .formatted(MAX_INVALID_ACTION_RETRIES + 1, stepNumber, skillName));
                        throw failure;
                    }
                    invalidActionFeedback = "Failed to parse model response as StepAction";
                    invalidActionRetryCount++;
                    continue;
                }
                lastAction = action;

                executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_ACTION_PROPOSED,
                        Map.of("stepAction", actionName(action),
                                "taskId", action.taskId() == null ? "" : action.taskId(),
                                "toolName", action.toolName() == null ? "" : action.toolName()),
                        Map.of());

                StepValidationResult validation = StepActionValidator.validate(action, plan, visibleTools, strictCompletion);
                boolean skillValidationRejected = false;
                if (validation.valid() && action.stepAction() == StepActionType.FINAL_RESPONSE)
                {
                    FinalResponseValidationOutcome finalValidation = validateFinalResponseForSkill(
                            session, action, skillDefinition, linterAttempt, outputSchemaAttempt, evidenceAttempt);
                    validation = finalValidation.validation();
                    linterAttempt = finalValidation.nextLinterAttempt();
                    outputSchemaAttempt = finalValidation.nextOutputSchemaAttempt();
                    evidenceAttempt = finalValidation.nextEvidenceAttempt();
                    skillValidationRejected = !validation.valid();
                    if (!validation.valid() && finalValidation.exhausted())
                    {
                        executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_ACTION_REJECTED,
                                Map.of("reason", validation.rejectionReason(), "exhausted", true),
                                Map.of("stepAction", actionName(action)));
                        IllegalStateException failure = new IllegalStateException("Final response validation exhausted at step %d for skill '%s': %s"
                                .formatted(stepNumber, skillName, validation.rejectionReason()));
                        throw failure;
                    }
                }
                if (!validation.valid())
                {
                    executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_ACTION_REJECTED,
                            Map.of("retry", invalidActionRetryCount, "reason", validation.rejectionReason()),
                            Map.of("stepAction", actionName(action)));
                    log.debug("Step {} action rejected (retry {}): {}", stepNumber, invalidActionRetryCount, validation.rejectionReason());
                    if (!skillValidationRejected && invalidActionRetryCount >= MAX_INVALID_ACTION_RETRIES)
                    {
                        IllegalStateException failure = new IllegalStateException("Step action validation exhausted at step %d for skill '%s': %s"
                                .formatted(stepNumber, skillName, validation.rejectionReason()));
                        throw failure;
                    }
                    invalidActionFeedback = validation.rejectionReason();
                    forceVerboseToolArgumentGuidance = true;
                    if (!skillValidationRejected)
                    {
                        invalidActionRetryCount++;
                    }
                    continue;
                }

                executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_ACTION_VALIDATED,
                        Map.of("stepAction", actionName(action)), Map.of());

                return switch (action.stepAction())
                {
                    case CALL_TOOL -> executeToolAction(session, action, skillDefinition, visibleTools, stepFrame, stepNumber);
                    case FINAL_RESPONSE -> {
                        String finalResponse = serializeFinalResponse(action.finalResponse());
                        executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_COMPLETED,
                                Map.of("stepAction", "FINAL_RESPONSE", "stepNumber", stepNumber), Map.of());
                        yield StepResult.finalResponse(finalResponse);
                    }
                };
            }
        }
        catch (RuntimeException | Error ex)
        {
            stepFailure = ex;
            boolean callerOwnsCleanup = cleanupOwner.get() == CleanupOwner.CALLER;
            boolean aborted = callerOwnsCleanup || Thread.currentThread().isInterrupted();
            stepFrameStatus = aborted ? "aborted" : "failed";
            if (!aborted)
            {
                String failureId = executionStateService.recordFailure(session, ex,
                        Map.of("message", "Step execution failed", "stepNumber", stepNumber, "skillName", skillName));
                LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
                metadata.put("stepNumber", stepNumber);
                metadata.put("failureId", failureId);
                metadata.put("exceptionType", ex.getClass().getName());
                metadata.put("message", truncate(ex.getMessage() == null ? "Step execution failed" : ex.getMessage(), 500));
                if (lastAction != null)
                {
                    metadata.put("stepAction", actionName(lastAction));
                    if (lastAction.taskId() != null) metadata.put("taskId", lastAction.taskId());
                    if (lastAction.toolName() != null) metadata.put("toolName", lastAction.toolName());
                }
                executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_FAILED,
                        metadata, Map.of("failureId", failureId));
            }
            throw ex;
        }
        finally
        {
            executionStateService.closeFrame(session, stepFrame, closeMetadata(stepFrameStatus, stepFailure));
        }
    }

    private String callModelForStep(LoomspanSession session,
            String skillName,
            EffectiveSkillExecutionConfiguration executionConfiguration,
            ModelInteraction modelInteraction,
            String stepPrompt,
            RenderedMissionInput renderedInput,
            int stepNumber,
            Map<String, Object> promptTraceMetadata,
            AtomicReference<CleanupOwner> cleanupOwner)
    {
        ModelExecutionIdentity modelIdentity = ModelExecutionIdentity.from(executionConfiguration);
        ExecutionFrame modelFrame = executionStateService.openFrame(
                session,
                TraceFrameType.MODEL_CALL,
                skillName + "#step-" + stepNumber + "-model",
                modelIdentity.metadata("segment", "step-" + stepNumber));

        String modelFrameStatus = "completed";
        Throwable modelFailure = null;
        try
        {
            ModelTraceContext modelTraceContext = new ModelTraceContext(
                    modelIdentity,
                    skillName,
                    "step-" + stepNumber);

            return modelInteraction.call(new ModelInteractionRequest(
                    stepPrompt, renderedInput, modelTraceContext, List.of(), false)).content();
        }
        catch (RuntimeException | Error ex)
        {
            modelFailure = ex;
            boolean callerOwnsCleanup = cleanupOwner.get() == CleanupOwner.CALLER;
            modelFrameStatus = callerOwnsCleanup || Thread.currentThread().isInterrupted() ? "aborted" : "failed";
            if (!callerOwnsCleanup)
            {
                executionStateService.recordFailure(session, ex, Map.of("message", "Step model invocation failed"));
            }
            throw ex;
        }
        finally
        {
            executionStateService.closeFrame(session, modelFrame, closeMetadata(modelFrameStatus, modelFailure));
        }
    }

    private Map<String, Object> buildStepTracePayload(String stepPrompt,
            RenderedMissionInput renderedInput,
            Map<String, Object> promptTraceMetadata)
    {
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("system", stepPrompt);
        payload.put("user", renderedInput.userText());
        payload.put("attachments", attachmentDescriptors(renderedInput));
        payload.put("attachmentCount", renderedInput.attachments().size());
        payload.putAll(promptTraceMetadata);
        return Map.copyOf(payload);
    }

    private StepResult executeToolAction(LoomspanSession session,
            StepAction action,
            @Nullable YamlSkillDefinition skillDefinition,
            List<BoundCapability> visibleTools,
            ExecutionFrame stepFrame,
            int stepNumber)
    {
        BoundCapability toolCallback = visibleTools.stream()
                .filter(t -> t != null && action.toolName().equals(t.name()))
                .findFirst()
                .orElseThrow(() -> new IllegalStateException(
                        "Tool '%s' validated but not found in visible tools".formatted(action.toolName())));

        planningService.markTaskStarted(session,
                action.taskId(),
                action.toolName(),
                action.toolArguments() == null ? Map.of() : action.toolArguments());

        String toolResult;
        try
        {
            Object rawResult = toolCallback.invoke(
                    action.toolArguments() == null ? Map.of() : action.toolArguments(), action.taskId());

            toolResult = rawResult == null ? "null" : String.valueOf(rawResult);
            planningService.markToolCompleted(
                    session,
                    action.taskId(),
                    action.toolName(),
                    rawResult);
        }
        catch (RuntimeException ex)
        {
            planningService.markToolFailed(session, action.taskId(), action.toolName(), ex);
            RuntimeException unwrapped = unwrapMissionFailure(ex);
            if (unwrapped instanceof LoomspanStackOverflowException || unwrapped instanceof LoomspanMissionTimeoutException)
            {
                throw unwrapped;
            }
            throw new IllegalStateException("Step %d tool '%s' failed for task '%s'"
                    .formatted(stepNumber, action.toolName(), action.taskId()), ex);
        }

        String summaryLine = "Step %d: Called %s for task %s -> %s".formatted(
                stepNumber,
                action.toolName(),
                action.taskId(),
                toolResult.length() > 100 ? toolResult.substring(0, 100) + "..." : toolResult);

        executionStateService.recordStepEvent(session, stepFrame, TraceRecordType.STEP_COMPLETED,
                Map.of("stepAction", "CALL_TOOL", "stepNumber", stepNumber,
                        "taskId", action.taskId(), "toolName", action.toolName()),
                Map.of("resultPreview", truncate(toolResult, 200)));

        return StepResult.toolExecuted(toolResult, summaryLine);
    }

    @Nullable
    private StepAction parseStepAction(String modelResponse, boolean finalResponseOnly)
    {
        if (modelResponse == null || modelResponse.isBlank())
        {
            return null;
        }
        String unwrapped = unwrapFencedBlock(modelResponse);
        try
        {
            StepAction parsed = objectMapper.readValue(unwrapped, StepAction.class);
            if (finalResponseOnly && (parsed.stepAction() == null || parsed.stepAction() != StepActionType.FINAL_RESPONSE))
            {
                JsonNode node = objectMapper.readTree(unwrapped);
                if (looksLikeBareFinalResponsePayload(node))
                {
                    return StepAction.finalResponse(node);
                }
            }
            return parsed;
        }
        catch (JacksonException ex)
        {
            log.debug("Failed to parse step action JSON: {}", ex.getMessage());
            return null;
        }
    }

    private boolean looksLikeBareFinalResponsePayload(@Nullable JsonNode node)
    {
        return node != null
                && node.isObject()
                && !node.has("stepAction")
                && !node.has("action")
                && !node.has("finalResponse");
    }

    private String unwrapFencedBlock(String payload)
    {
        String safePayload = payload.trim();
        if (safePayload.startsWith("```"))
        {
            int firstNewline = safePayload.indexOf('\n');
            int lastFence = safePayload.lastIndexOf("```");
            if (firstNewline >= 0 && lastFence > firstNewline)
            {
                return safePayload.substring(firstNewline + 1, lastFence).trim();
            }
        }
        return safePayload;
    }

    private void recordTerminalFailure(LoomspanSession session,
            String skillName,
            int stepNumber,
            Throwable failure,
            String message)
    {
        executionStateService.recordFailure(session, failure, Map.of(
                "skillName", skillName,
                "stepNumber", stepNumber,
                "phase", "step-loop",
                "message", message));
    }

    private void validatePlanForStepLoop(LoomspanSession session, String skillName, ExecutionPlan plan)
    {
        List<String> duplicateTaskIds = plan.tasks().stream()
                .map(PlanTask::taskId)
                .collect(Collectors.groupingBy(Function.identity(), LinkedHashMap::new, Collectors.counting()))
                .entrySet().stream()
                .filter(entry -> entry.getValue() > 1)
                .map(Map.Entry::getKey)
                .toList();

        List<String> unsupportedTaskIds = plan.tasks().stream()
                .filter(PlanTask::autoCompletable)
                .map(PlanTask::taskId)
                .toList();

        List<String> invalidDependencies = plan.tasks().stream()
                .flatMap(task -> task.dependsOn().stream()
                        .filter(dependencyId -> plan.findTask(dependencyId).isEmpty())
                        .map(dependencyId -> task.taskId() + "->" + dependencyId))
                .toList();

        List<String> unboundToolTasks = plan.tasks().stream()
                .filter(task -> !task.autoCompletable())
                .filter(task -> task.capabilityName() == null || task.capabilityName().isBlank())
                .map(PlanTask::taskId)
                .toList();

        if (duplicateTaskIds.isEmpty() && unsupportedTaskIds.isEmpty() && invalidDependencies.isEmpty()
                && unboundToolTasks.isEmpty())
        {
            return;
        }

        StringBuilder message = new StringBuilder("Step-loop execution rejected an invalid Phase 6 plan.");
        if (!duplicateTaskIds.isEmpty())
        {
            message.append(" Task IDs must be unique. Duplicates: ").append(duplicateTaskIds).append(".");
        }
        if (!unsupportedTaskIds.isEmpty())
        {
            message.append(" autoCompletable tasks are not supported: ").append(unsupportedTaskIds).append(".");
        }
        if (!invalidDependencies.isEmpty())
        {
            message.append(" Missing task dependencies: ").append(invalidDependencies).append(".");
        }
        if (!unboundToolTasks.isEmpty())
        {
            message.append(" Tasks missing capability bindings: ").append(unboundToolTasks).append(".");
        }

        IllegalStateException failure = new IllegalStateException(message.toString());
        recordTerminalFailure(session, skillName, 0, failure, message.toString());
        throw failure;
    }

    private FinalResponseValidationOutcome validateFinalResponseForSkill(LoomspanSession session,
            StepAction action,
            @Nullable YamlSkillDefinition skillDefinition,
            int linterAttempt,
            int outputSchemaAttempt,
            int evidenceAttempt)
    {
        if (skillDefinition == null)
        {
            return FinalResponseValidationOutcome.ok(linterAttempt, outputSchemaAttempt, evidenceAttempt);
        }

        String finalResponse = serializeFinalResponse(action.finalResponse());
        ValidatorAttemptOutcome outputSchemaValidation = validateOutputSchema(
                session, skillDefinition, finalResponse, outputSchemaAttempt);
        if (!outputSchemaValidation.valid())
        {
            return new FinalResponseValidationOutcome(
                    outputSchemaValidation.validation(),
                    linterAttempt,
                    outputSchemaValidation.nextAttempt(),
                    evidenceAttempt,
                    outputSchemaValidation.exhausted());
        }

        ValidatorAttemptOutcome evidenceValidation = validateEvidence(session, skillDefinition, action.finalResponse(), evidenceAttempt);
        if (!evidenceValidation.valid())
        {
            return new FinalResponseValidationOutcome(
                    evidenceValidation.validation(),
                    linterAttempt,
                    outputSchemaAttempt,
                    evidenceValidation.nextAttempt(),
                    evidenceValidation.exhausted());
        }

        ValidatorAttemptOutcome linterValidation = validateLinter(skillDefinition, finalResponse, linterAttempt);
        return new FinalResponseValidationOutcome(
                linterValidation.validation(),
                linterValidation.nextAttempt(),
                outputSchemaAttempt,
                evidenceAttempt,
                linterValidation.exhausted());
    }

    private ValidatorAttemptOutcome validateLinter(YamlSkillDefinition skillDefinition,
            @Nullable String finalResponse,
            int attempt)
    {
        var linter = skillDefinition.linter();
        if (linter == null || !"regex".equals(linter.getType()) || linter.getRegex() == null)
        {
            return ValidatorAttemptOutcome.passed(attempt);
        }

        int maxRetries = linter.getMaxRetries() == null ? 0 : linter.getMaxRetries();
        boolean matches = finalResponse != null && java.util.regex.Pattern
                .compile(linter.getRegex().getPattern())
                .matcher(finalResponse)
                .matches();
        LinterOutcomeStatus status = matches
                ? LinterOutcomeStatus.PASSED
                : attempt <= maxRetries ? LinterOutcomeStatus.RETRYING : LinterOutcomeStatus.EXHAUSTED;
        String detail = matches
                ? "Final response matched configured regex linter."
                : (linter.getRegex().getMessage() == null || linter.getRegex().getMessage().isBlank()
                        ? "Final response did not match the configured regex linter."
                        : linter.getRegex().getMessage());

        executionStateService.recordLinterOutcome(LoomspanSession.getCurrentSession(), new LinterOutcome(
                skillDefinition.manifest().getName(),
                linter.getType(),
                attempt,
                attempt - 1,
                maxRetries,
                status,
                detail));

        return matches
                ? ValidatorAttemptOutcome.passed(attempt)
                : ValidatorAttemptOutcome.failed(detail, attempt + 1, status == LinterOutcomeStatus.EXHAUSTED);
    }

    private ValidatorAttemptOutcome validateOutputSchema(LoomspanSession session,
            YamlSkillDefinition skillDefinition,
            @Nullable String finalResponse,
            int attempt)
    {
        if (skillDefinition.outputSchema() == null)
        {
            return ValidatorAttemptOutcome.passed(attempt);
        }

        int maxRetries = skillDefinition.outputSchemaMaxRetries();
        OutputSchemaValidationResult result = outputSchemaValidator.validate(finalResponse, skillDefinition.outputSchema());
        OutputSchemaOutcomeStatus status = result.valid()
                ? OutputSchemaOutcomeStatus.PASSED
                : attempt <= maxRetries ? OutputSchemaOutcomeStatus.RETRYING : OutputSchemaOutcomeStatus.EXHAUSTED;

        executionStateService.recordOutputSchemaOutcome(session, new OutputSchemaOutcome(
                skillDefinition.manifest().getName(),
                result.failureMode(),
                attempt,
                attempt - 1,
                maxRetries,
                status,
                result.issues()));

        if (result.valid())
        {
            return ValidatorAttemptOutcome.passed(attempt);
        }

        return ValidatorAttemptOutcome.failed(
                "Final response violates output_schema: " + summarizeOutputSchemaIssues(result.issues()),
                attempt + 1,
                status == OutputSchemaOutcomeStatus.EXHAUSTED);
    }

    private ValidatorAttemptOutcome validateEvidence(LoomspanSession session,
            YamlSkillDefinition skillDefinition,
            @Nullable JsonNode finalResponseNode,
            int attempt)
    {
        if (skillDefinition.evidenceContract().isEmpty())
        {
            return ValidatorAttemptOutcome.passed(attempt);
        }

        int maxRetries = skillDefinition.outputSchemaMaxRetries();
        EvidenceCoverageResult result = evidenceBackedOutputValidator.validate(
                finalResponseNode,
                skillDefinition.evidenceContract(),
                executionStateService.currentSuccessfulSkills(session));

        executionStateService.recordEvidenceValidation(
                session,
                result.complete(),
                Map.of(
                        "skillName", skillDefinition.manifest().getName(),
                        "claims", result.evaluatedClaims(),
                        "attempt", attempt,
                        "maxRetries", maxRetries),
                result);

        if (result.complete())
        {
            return ValidatorAttemptOutcome.passed(attempt);
        }

        boolean exhausted = attempt > maxRetries;
        if (exhausted)
        {
            return ValidatorAttemptOutcome.failed(
                    "Final response is unsupported by gathered evidence: " + result.retryFeedback(),
                    attempt,
                    true);
        }

        return ValidatorAttemptOutcome.failed(
                "Final response is unsupported by gathered evidence: " + result.retryFeedback(),
                attempt + 1,
                false);
    }

    private String actionName(StepAction action)
    {
        if (action == null || action.stepAction() == null)
        {
            return "";
        }
        return action.stepAction().name();
    }

    private String serializeFinalResponse(@Nullable tools.jackson.databind.JsonNode finalResponseNode)
    {
        if (finalResponseNode == null || finalResponseNode.isNull())
        {
            return "";
        }
        if (finalResponseNode.isTextual())
        {
            return finalResponseNode.asText();
        }
        try
        {
            return objectMapper.writeValueAsString(finalResponseNode);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException("Failed to serialize FINAL_RESPONSE payload", ex);
        }
    }

    private String summarizeOutputSchemaIssues(List<OutputSchemaValidationIssue> issues)
    {
        if (issues == null || issues.isEmpty())
        {
            return "unknown schema validation error";
        }

        return issues.stream()
                .limit(3)
                .map(issue ->
                {
                    String field = issue.canonicalField() == null || issue.canonicalField().isBlank()
                            ? issue.path()
                            : issue.canonicalField();
                    return field + ": " + issue.message();
                })
                .reduce((left, right) -> left + "; " + right)
                .orElse("unknown schema validation error");
    }

    @Nullable
    private String formatExecutionSummary(Deque<String> executionSummary)
    {
        if (executionSummary == null || executionSummary.isEmpty())
        {
            return null;
        }

        return String.join("\n", executionSummary);
    }

    private void appendExecutionSummary(Deque<String> executionSummary, String summaryLine)
    {
        if (summaryLine == null || summaryLine.isBlank())
        {
            return;
        }

        executionSummary.addLast(summaryLine);

        while (executionSummary.size() > MAX_EXECUTION_SUMMARY_LINES)
        {
            executionSummary.removeFirst();
        }
    }

    private List<Map<String, Object>> attachmentDescriptors(RenderedMissionInput renderedInput)
    {
        return renderedInput.attachments().stream()
                .map(attachment -> attachment.descriptor())
                .toList();
    }

    @Nullable
    private Map<String, Object> planningInput(@Nullable Map<String, Object> originalInput, RenderedMissionInput renderedInput)
    {
        return renderedInput.attachments().isEmpty() ? originalInput : renderedInput.traceSafeInput();
    }

    private static MissionInputMaterializer defaultMaterializer()
    {
        return new DefaultMissionInputMaterializer(new DefaultRefResolver(
                new SessionLocalVirtualFileSystem(Paths.get(System.getProperty("java.io.tmpdir"), "loomspan-vfs"))));
    }

    private void unwindMissionFrames(LoomspanSession session, int baselineFrameDepth)
    {
        while (session.getFramesSnapshot().size() > baselineFrameDepth)
        {
            ExecutionFrame activeFrame = session.peekFrame();
            executionStateService.closeFrame(session, activeFrame, Map.of(
                    "status", "aborted",
                    "reason", "mission-cleanup"));
        }
    }

    private RuntimeException unwrapMissionFailure(RuntimeException runtimeException)
    {
        return runtimeException;
    }

    private void awaitMissionCleanup(LoomspanSession session,
            Future<String> mission,
            int baselineFrameDepth,
            AtomicReference<CleanupOwner> cleanupOwner,
            CountDownLatch cleanupComplete,
            boolean callerOwnsCleanup)
    {
        if (callerOwnsCleanup)
        {
            try
            {
                cleanupComplete.await(250, TimeUnit.MILLISECONDS);
            }
            catch (InterruptedException ex)
            {
                Thread.currentThread().interrupt();
            }
            unwindMissionFrames(session, baselineFrameDepth);
            return;
        }

        try
        {
            mission.get(250, TimeUnit.MILLISECONDS);
        }
        catch (InterruptedException ex)
        {
            Thread.currentThread().interrupt();
        }
        catch (ExecutionException | TimeoutException | java.util.concurrent.CancellationException ignored)
        {
            // Best effort wait only.
        }
        if (cleanupComplete.getCount() == 0L)
        {
            return;
        }
        if (cleanupOwner.compareAndSet(CleanupOwner.NONE, CleanupOwner.CALLER))
        {
            try
            {
                unwindMissionFrames(session, baselineFrameDepth);
            }
            finally
            {
                cleanupComplete.countDown();
            }
            return;
        }
        try
        {
            cleanupComplete.await(250, TimeUnit.MILLISECONDS);
        }
        catch (InterruptedException ex)
        {
            Thread.currentThread().interrupt();
        }
    }

    private Map<String, Object> closeMetadata(String status, @Nullable Throwable failure)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("status", Thread.currentThread().isInterrupted() ? "aborted" : status);
        if (failure != null)
        {
            TraceFailureMetadata.addTo(metadata, failure, "Step execution failed");
        }
        return metadata;
    }

    private static String truncate(@Nullable String value, int maxLength)
    {
        if (value == null)
        {
            return "";
        }
        return value.length() <= maxLength ? value : value.substring(0, maxLength) + "...";
    }

    private record StepResult(@Nullable String finalResponse, @Nullable String toolResult, @Nullable String summaryLine)
    {
        boolean isFinalResponse()
        {
            return finalResponse != null;
        }

        static StepResult finalResponse(String response)
        {
            return new StepResult(response, null, null);
        }

        static StepResult toolExecuted(String toolResult, String summaryLine)
        {
            return new StepResult(null, toolResult, summaryLine);
        }
    }

    private record FinalResponseValidationOutcome(StepValidationResult validation,
            int nextLinterAttempt,
            int nextOutputSchemaAttempt,
            int nextEvidenceAttempt,
            boolean exhausted)
    {
        static FinalResponseValidationOutcome ok(int linterAttempt, int outputSchemaAttempt, int evidenceAttempt)
        {
            return new FinalResponseValidationOutcome(
                    StepValidationResult.ok(),
                    linterAttempt,
                    outputSchemaAttempt,
                    evidenceAttempt,
                    false);
        }
    }

    private record ValidatorAttemptOutcome(StepValidationResult validation, int nextAttempt, boolean exhausted)
    {
        boolean valid()
        {
            return validation.valid();
        }

        static ValidatorAttemptOutcome passed(int attempt)
        {
            return new ValidatorAttemptOutcome(StepValidationResult.ok(), attempt, false);
        }

        static ValidatorAttemptOutcome failed(String reason, int nextAttempt, boolean exhausted)
        {
            return new ValidatorAttemptOutcome(StepValidationResult.rejected(reason), nextAttempt, exhausted);
        }
    }
}
