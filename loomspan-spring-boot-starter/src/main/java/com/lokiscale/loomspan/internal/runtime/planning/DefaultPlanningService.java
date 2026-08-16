package com.lokiscale.loomspan.internal.runtime.planning;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;
import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.ModelTraceContext;
import com.lokiscale.loomspan.internal.core.ModelExecutionIdentity;
import com.lokiscale.loomspan.internal.core.MissionInputMessageFormatter;
import com.lokiscale.loomspan.internal.core.PlanStatus;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskLinker;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceFailureMetadata;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageResult;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageValidator;
import com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposer;
import com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposition;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import com.lokiscale.loomspan.internal.model.ModelInteraction;
import com.lokiscale.loomspan.internal.model.ModelInteractionRequest;
import com.lokiscale.loomspan.internal.model.ModelInteractionResult;
import com.lokiscale.loomspan.internal.runtime.attachment.RenderedMissionInput;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.springframework.lang.Nullable;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.function.Function;
import java.util.function.Supplier;
import java.util.stream.Collectors;

public class DefaultPlanningService implements PlanningService
{
    private static final Logger log = LoggerFactory.getLogger(DefaultPlanningService.class);

    private static final int MAX_PLAN_QUALITY_RETRIES = 1;

    private final PlanTaskLinker planTaskLinker;
    private final ExecutionStateService executionStateService;
    private final ObjectMapper objectMapper;
    private final ObjectMapper yamlObjectMapper;
    private final PlanQualityValidator planQualityValidator;
    private final EvidenceCoverageValidator evidenceCoverageValidator;
    private final Supplier<String> planIdSupplier;

    public DefaultPlanningService(PlanTaskLinker planTaskLinker, ExecutionStateService executionStateService)
    {
        this(
                planTaskLinker,
                executionStateService,
                defaultObjectMapper(),
                defaultYamlObjectMapper(),
                new PlanQualityValidator(),
                new EvidenceCoverageValidator(),
                () -> UUID.randomUUID().toString());
    }

    public DefaultPlanningService(PlanTaskLinker planTaskLinker,
            ExecutionStateService executionStateService,
            ObjectMapper planningJsonMapper,
            ObjectMapper planningYamlMapper)
    {
        this(planTaskLinker, executionStateService, planningJsonMapper, planningYamlMapper,
                new PlanQualityValidator(), new EvidenceCoverageValidator(),
                () -> UUID.randomUUID().toString());
    }

    DefaultPlanningService(PlanTaskLinker planTaskLinker,
            ExecutionStateService executionStateService,
            ObjectMapper objectMapper,
            ObjectMapper yamlObjectMapper,
            PlanQualityValidator planQualityValidator,
            EvidenceCoverageValidator evidenceCoverageValidator,
            Supplier<String> planIdSupplier)
    {
        this.planTaskLinker = Objects.requireNonNull(planTaskLinker, "planTaskLinker must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
        this.yamlObjectMapper = Objects.requireNonNull(yamlObjectMapper, "yamlObjectMapper must not be null");
        this.planQualityValidator = Objects.requireNonNull(planQualityValidator, "planQualityValidator must not be null");
        this.evidenceCoverageValidator = Objects.requireNonNull(evidenceCoverageValidator, "evidenceCoverageValidator must not be null");
        this.planIdSupplier = Objects.requireNonNull(planIdSupplier, "planIdSupplier must not be null");
    }

    @Override
    public Optional<ExecutionPlan> initializePlan(LoomspanSession session,
            String objective,
            @Nullable Map<String, Object> missionInput,
            YamlSkillDefinition definition,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(objective, "objective must not be null");
        Objects.requireNonNull(definition, "definition must not be null");
        Objects.requireNonNull(modelInteraction, "modelInteraction must not be null");
        String capabilityName = definition.manifest().getName();
        var executionConfiguration = definition.requireExecutionConfiguration();

        log.debug(
                "Initializing plan for capability='{}' chatClientType={} visibleTools={}",
                capabilityName,
                modelInteraction.getClass().getName(),
                visibleTools == null ? 0 : visibleTools.size());

        ModelExecutionIdentity modelIdentity = ModelExecutionIdentity.from(executionConfiguration);
        ExecutionFrame planningFrame = executionStateService.openFrame(
                session,
                TraceFrameType.PLANNING,
                capabilityName + "#planning",
                modelIdentity.metadata());

        String planningFrameStatus = "completed";
        Throwable planningFailure = null;
        try
        {
            return initializePlanWithQualityChecks(
                    session,
                    objective,
                    missionInput,
                    definition,
                    modelInteraction,
                    visibleTools,
                    planningFrame);
        }
        catch (RuntimeException | Error ex)
        {
            planningFailure = ex;
            planningFrameStatus = Thread.currentThread().isInterrupted() ? "aborted" : "failed";
            executionStateService.recordFailure(session, ex, Map.of("message", "Planning failed"));
            throw ex;
        }
        finally
        {
            executionStateService.closeFrame(session, planningFrame, closeMetadata(planningFrameStatus, planningFailure));
        }
    }

    @Override
    public Optional<ExecutionPlan> markToolStarted(LoomspanSession session, CapabilityMetadata capability, Map<String, Object> arguments)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(capability, "capability must not be null");
        Map<String, Object> safeArguments = arguments == null ? Map.of() : Map.copyOf(arguments);

        return executionStateService.currentPlan(session)
                .flatMap(plan -> planTaskLinker.linkTask(plan, capability, safeArguments)
                        .map(taskId ->
                        {
                            ExecutionPlan updated = replacePlanTask(
                                    plan,
                                    taskId,
                                    task -> task.bindInProgress("Starting tool " + capability.name()))
                                            .withActiveTask(taskId);
                            executionStateService.storePlan(session, updated);
                            executionStateService.logPlanUpdated(session, updated);
                            return updated;
                        }));
    }

    @Override
    public Optional<ExecutionPlan> markTaskStarted(LoomspanSession session,
            String taskId,
            String capabilityName,
            @Nullable Map<String, Object> arguments)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(taskId, "taskId must not be null");
        Objects.requireNonNull(capabilityName, "capabilityName must not be null");

        return executionStateService.currentPlan(session)
                .flatMap(plan -> plan.findTask(taskId)
                        .map(task ->
                        {
                            requireBoundCapability(task, capabilityName);
                            ExecutionPlan updated = replacePlanTask(
                                    plan,
                                    taskId,
                                    current -> current.bindInProgress("Starting tool " + capabilityName))
                                            .withActiveTask(taskId);
                            executionStateService.storePlan(session, updated);
                            executionStateService.logPlanUpdated(session, updated);
                            return updated;
                        }));
    }

    @Override
    public Optional<ExecutionPlan> markToolCompleted(LoomspanSession session,
            String taskId,
            String capabilityName,
            @Nullable Object result)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(taskId, "taskId must not be null");
        Objects.requireNonNull(capabilityName, "capabilityName must not be null");

        Optional<ExecutionPlan> updatedPlan = executionStateService.currentPlan(session)
                .flatMap(plan -> plan.findTask(taskId)
                        .map(task ->
                        {
                            requireBoundCapability(task, capabilityName);
                            ExecutionPlan updated = replacePlanTask(
                                    plan,
                                    taskId,
                                    current -> current.complete("Completed tool " + capabilityName))
                                            .clearActiveTask();
                            executionStateService.storePlan(session, updated);
                            executionStateService.logPlanUpdated(session, updated);
                            return updated;
                        }))
                .filter(plan -> plan.findTask(taskId)
                        .map(task -> task.status() == PlanTaskStatus.COMPLETED)
                        .orElse(false));

        if (updatedPlan.isPresent())
        {
            executionStateService.recordSuccessfulSkill(session, capabilityName, taskId, false);
        }
        return updatedPlan;
    }

    @Override
    public Optional<ExecutionPlan> markToolFailed(LoomspanSession session,
            String taskId,
            String capabilityName,
            RuntimeException ex)
    {
        Objects.requireNonNull(session, "session must not be null");
        Objects.requireNonNull(taskId, "taskId must not be null");
        Objects.requireNonNull(capabilityName, "capabilityName must not be null");
        Objects.requireNonNull(ex, "ex must not be null");

        return executionStateService.currentPlan(session)
                .flatMap(plan -> plan.findTask(taskId)
                        .map(task ->
                        {
                            requireBoundCapability(task, capabilityName);
                            ExecutionPlan updated = replacePlanTask(
                                    plan,
                                    taskId,
                                    current -> current.block("Tool " + capabilityName + " failed: " + ex.getClass().getSimpleName()))
                                            .withStatus(PlanStatus.STALE)
                                            .clearActiveTask();
                            executionStateService.storePlan(session, updated);
                            executionStateService.logPlanUpdated(session, updated);
                            return updated;
                        }));
    }

    private Optional<ExecutionPlan> initializePlanWithQualityChecks(LoomspanSession session,
            String objective,
            @Nullable Map<String, Object> missionInput,
            YamlSkillDefinition definition,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools,
            ExecutionFrame planningFrame)
    {
        String capabilityName = definition.manifest().getName();
        var executionConfiguration = definition.requireExecutionConfiguration();
        EvidenceContract evidenceContract = definition.evidenceContract();
        String retryFeedback = null;
        int retryCount = 0;
        ModelTraceContext modelTraceContext = new ModelTraceContext(
                ModelExecutionIdentity.from(executionConfiguration),
                capabilityName,
                "planning");

        while (true)
        {
            PlanningAttemptResult attemptResult = requestPlanAttempt(
                    session,
                    objective,
                    missionInput,
                    definition,
                    executionConfiguration,
                    modelInteraction,
                    visibleTools,
                    retryFeedback,
                    evidenceContract,
                    modelTraceContext);

            PlanQualityValidationResult validation = planQualityValidator.validate(attemptResult.plan(), visibleTools);
            EvidenceCoverageResult evidenceCoverage = evidenceCoverageValidator.validatePlanCoverage(
                    attemptResult.plan(),
                    evidenceContract);

            boolean hasDeterministicEvidenceGap = !evidenceCoverage.complete();
            if ((validation.hasErrors() || hasDeterministicEvidenceGap) && retryCount < MAX_PLAN_QUALITY_RETRIES)
            {
                recordPlanQualityEvent(session, planningFrame, TraceRecordType.PLAN_VALIDATION_FAILED,
                        validation.errors(), retryCount, attemptResult.modelAttempt());
                if (hasDeterministicEvidenceGap)
                {
                    recordEvidenceCoverageEvent(session, planningFrame, TraceRecordType.PLAN_VALIDATION_FAILED,
                            evidenceCoverage, retryCount, attemptResult.modelAttempt());
                }

                retryFeedback = mergeRetryFeedback(validation.retryFeedback(), evidenceCoverage.retryFeedback());
                recordPlanQualityEvent(session, planningFrame, TraceRecordType.PLAN_RETRY_REQUESTED,
                        validation.errors(), retryCount, attemptResult.modelAttempt());
                if (hasDeterministicEvidenceGap)
                {
                    recordEvidenceCoverageEvent(session, planningFrame, TraceRecordType.PLAN_RETRY_REQUESTED,
                            evidenceCoverage, retryCount, attemptResult.modelAttempt());
                }

                retryCount++;
                continue;
            }

            if (hasDeterministicEvidenceGap)
            {
                recordEvidenceCoverageEvent(session, planningFrame, TraceRecordType.PLAN_VALIDATION_FAILED,
                        evidenceCoverage, retryCount, attemptResult.modelAttempt());
                throw new IllegalStateException(
                        "Evidence coverage validation failed for skill '%s': %s"
                                .formatted(capabilityName, evidenceCoverage.retryFeedback()));
            }

            if (validation.hasWarnings())
            {
                recordPlanQualityEvent(session, planningFrame, TraceRecordType.PLAN_QUALITY_WARNING,
                        validation.warnings(), retryCount, attemptResult.modelAttempt());
            }

            if (validation.hasErrors())
            {
                recordPlanQualityEvent(session, planningFrame, TraceRecordType.PLAN_QUALITY_WARNING,
                        validation.errors(), retryCount, attemptResult.modelAttempt());
            }

            Map<String, Object> acceptedAttempt = requireAcceptedAttempt(attemptResult.modelAttempt());
            executionStateService.storePlan(session, attemptResult.plan());
            executionStateService.logPlanCreated(session, attemptResult.plan(), acceptedAttempt);
            return Optional.of(attemptResult.plan());
        }
    }

    private void recordEvidenceCoverageEvent(LoomspanSession session,
            ExecutionFrame planningFrame,
            TraceRecordType recordType,
            EvidenceCoverageResult coverage,
            int retryCount,
            Map<String, Object> modelAttempt)
    {
        if (coverage == null || coverage.complete())
        {
            return;
        }

        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("retryCount", retryCount);
        metadata.put("issueCodes", List.of("evidence-coverage"));
        metadata.put("severity", "ERROR");
        metadata.put("claims", coverage.evaluatedClaims());
        metadata.put("unsatisfiedClaims", coverage.issues().stream().map(com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageIssue::claimName).toList());
        metadata.put("requiredExpressions", coverage.requiredExpressions());
        metadata.put("satisfiedSkills", coverage.satisfiedSkills());
        metadata.put("unsatisfiedRequirements", coverage.issues().stream()
                .flatMap(issue -> issue.unsatisfiedRequirements().stream())
                .toList());
        metadata.putAll(modelAttempt);

        executionStateService.recordPlanningEvent(session, planningFrame, recordType, metadata, coverage.issues());
    }

    private PlanningAttemptResult requestPlanAttempt(LoomspanSession session,
            String objective,
            @Nullable Map<String, Object> missionInput,
            YamlSkillDefinition definition,
            EffectiveSkillExecutionConfiguration executionConfiguration,
            ModelInteraction modelInteraction,
            List<BoundCapability> visibleTools,
            @Nullable String retryFeedback,
            @Nullable EvidenceContract evidenceContract,
            ModelTraceContext modelTraceContext)
    {
        String capabilityName = definition.manifest().getName();
        ModelExecutionIdentity modelIdentity = ModelExecutionIdentity.from(executionConfiguration);
        ExecutionFrame modelFrame = executionStateService.openFrame(
                session,
                TraceFrameType.MODEL_CALL,
                capabilityName + "#planning-model",
                modelIdentity.metadata("segment", "planning"));

        String modelFrameStatus = "completed";
        Throwable modelFailure = null;
        SkillPromptComposition promptComposition = SkillPromptComposer.composePlanningPrompt(
                definition,
                buildPlanningPrompt(capabilityName, visibleTools, retryFeedback, evidenceContract));
        String planningPrompt = promptComposition.systemPrompt();
        String planningUserMessage = MissionInputMessageFormatter.buildUserMessage(objective, missionInput);

        try
        {
            ModelInteractionResult result = modelInteraction.call(new ModelInteractionRequest(
                    planningPrompt,
                    new RenderedMissionInput(planningUserMessage, List.of(), Map.of()),
                    modelTraceContext,
                    List.of(),
                    true));
            String planPayload = result.content();
            Map<String, Object> modelAttempt = ModelTraceContext.attemptFrom(result.context());

            return new PlanningAttemptResult(
                    parsePlan(planPayload, capabilityName),
                    planningPrompt,
                    planningUserMessage,
                    modelAttempt);
        }
        catch (RuntimeException | Error ex)
        {
            modelFailure = ex;
            modelFrameStatus = Thread.currentThread().isInterrupted() ? "aborted" : "failed";
            executionStateService.recordFailure(session, ex, Map.of("message", "Planning model invocation failed"));
            throw ex;
        }
        finally
        {
            executionStateService.closeFrame(session, modelFrame, closeMetadata(modelFrameStatus, modelFailure));
        }
    }

    private Map<String, Object> buildPlanningTracePayload(SkillPromptComposition composition, String userMessage)
    {
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("system", composition.systemPrompt());
        payload.put("user", userMessage);
        payload.putAll(composition.traceMetadata());
        return Map.copyOf(payload);
    }

    private void recordPlanQualityEvent(LoomspanSession session,
            ExecutionFrame planningFrame,
            TraceRecordType recordType,
            List<PlanQualityIssue> issues,
            int retryCount,
            Map<String, Object> modelAttempt)
    {
        if (issues == null || issues.isEmpty())
        {
            return;
        }

        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("retryCount", retryCount);
        metadata.put("severity", issues.getFirst().severity().name());
        metadata.put("issueCodes", issues.stream().map(PlanQualityIssue::code).distinct().toList());
        metadata.putAll(modelAttempt);
        List<Map<String, Object>> payload = issues.stream()
                .map(issue -> Map.<String, Object>of(
                        "code", issue.code(),
                        "severity", issue.severity().name(),
                        "message", issue.message()))
                .toList();

        executionStateService.recordPlanningEvent(session, planningFrame, recordType, metadata, payload);
    }

    private static String buildPlanningPrompt(String capabilityName,
            List<BoundCapability> visibleTools,
            @Nullable String retryFeedback,
            @Nullable EvidenceContract evidenceContract)
    {
        String toolList = (visibleTools == null || visibleTools.isEmpty())
                ? "(none)"
                : visibleTools.stream()
                        .filter(Objects::nonNull)
                        .map(DefaultPlanningService::describeTool)
                        .collect(Collectors.joining("\n"));

        String retrySection = retryFeedback == null || retryFeedback.isBlank()
                ? ""
                : """

                        Previous plan was too weak. Correct these issues in the next plan:
                        %s
                        """.formatted(retryFeedback);

        String evidenceConstraints = "";
        if (evidenceContract != null && !evidenceContract.isEmpty())
        {
            StringBuilder builder = new StringBuilder();
            List<String> sortedClaims = new ArrayList<>(evidenceContract.claims());
            Collections.sort(sortedClaims);

            for (String claim : sortedClaims)
            {
                builder.append("- The '")
                        .append(claim)
                        .append("' output field requires tasks whose exact capability names satisfy: ")
                        .append(evidenceContract.canonicalExpressionForClaim(claim))
                        .append(". For an 'or' group, include any one alternative; for an 'and' group, include every requirement.\n");
            }

            if (!builder.isEmpty())
            {
                evidenceConstraints = "Evidence Constraints:\n" + builder.toString();
            }
        }

        return """
                Create an ordered flight plan for this mission before execution.
                Return ONLY valid JSON - no markdown, no explanation, no code fences.
                The JSON must match this exact structure:
                {
                  "capabilityName": "%s",
                  "createdAt": "<ISO-8601 timestamp, e.g. 2024-01-01T00:00:00Z>",
                  "status": "VALID",
                  "activeTaskId": null,
                  "tasks": [
                    {
                      "taskId": "<unique string>",
                      "title": "<short title>",
                      "status": "PENDING",
                      "capabilityName": "<one of the available sub-skills listed below>",
                      "intent": "<what this task must accomplish>",
                      "dependsOn": [],
                      "expectedOutputs": ["<output description>"],
                      "autoCompletable": false,
                      "note": "<optional note or empty string>"
                    }
                  ]
                }
                Available sub-skills (use these exact names for task capabilityName):
                %s
                Constraints:
                - plan status must be exactly: VALID
                - task status must be exactly: PENDING
                - autoCompletable must be a boolean (true or false)
                - dependsOn must be a JSON array of taskId strings (empty array if no dependencies)
                - expectedOutputs must be a JSON array of strings
                - activeTaskId must be null
                - Return raw JSON only - no additional text before or after
                Planning quality rules:
                - Each task must have a distinct purpose that advances the mission.
                - Bind each task to the tool that best matches that task's intent.
                - If multiple tools are available, consider whether the mission requires evidence from more than one.
                - Do not create report or conclusion tasks that are bound to extraction-only or lookup-only tools unless that tool is genuinely the right fit.
                - Gather enough evidence to support the final answer before the mission is complete.
                %s%s""".formatted(capabilityName, toolList, evidenceConstraints, retrySection);
    }

    private String mergeRetryFeedback(@Nullable String qualityFeedback, @Nullable String evidenceFeedback)
    {
        if ((qualityFeedback == null || qualityFeedback.isBlank())
                && (evidenceFeedback == null || evidenceFeedback.isBlank()))
        {
            return null;
        }
        if (qualityFeedback == null || qualityFeedback.isBlank())
        {
            return evidenceFeedback;
        }
        if (evidenceFeedback == null || evidenceFeedback.isBlank())
        {
            return qualityFeedback;
        }
        return qualityFeedback + "\n" + evidenceFeedback;
    }

    private static String describeTool(BoundCapability callback)
    {
        String description = callback.description();

        if (description == null || description.isBlank())
        {
            description = "No description provided.";
        }

        return "- %s: %s".formatted(callback.name(), description);
    }

    private Map<String, Object> closeMetadata(String status, @Nullable Throwable failure)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("status", Thread.currentThread().isInterrupted() ? "aborted" : status);

        if (failure != null)
        {
            TraceFailureMetadata.addTo(metadata, failure, "Planning model invocation failed");
        }

        return metadata;
    }

    private ExecutionPlan replacePlanTask(ExecutionPlan plan, String taskId, Function<PlanTask, PlanTask> updater)
    {
        return plan.updateTask(taskId, updater);
    }

    private ExecutionPlan parsePlan(String payload, String capabilityName)
    {
        String unwrapped = unwrapFencedBlock(payload);
        log.debug(
                "Parsing plan for capability='{}' looksLikeJson={} payloadPreview={}...",
                capabilityName,
                looksLikeJson(unwrapped),
                preview(unwrapped));

        try
        {
            JsonNode tree = parsePlanTree(unwrapped, capabilityName);
            normalizePlanTree(tree);
            return objectMapper.treeToValue(tree, ExecutionPlan.class);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException(
                    "Failed to parse planning response for capability '" + capabilityName
                            + "' as JSON or YAML. Payload preview: " + preview(unwrapped),
                    ex);
        }
    }

    private void requireBoundCapability(PlanTask task, String capabilityName)
    {
        if (!Objects.equals(task.capabilityName(), capabilityName))
        {
            throw new IllegalStateException(
                    "Task '%s' is bound to capability '%s' but received '%s'."
                            .formatted(task.taskId(), task.capabilityName(), capabilityName));
        }
    }

    private JsonNode parsePlanTree(String payload, String capabilityName) throws JacksonException
    {
        if (looksLikeJson(payload))
        {
            try
            {
                return objectMapper.readTree(payload);
            }
            catch (JacksonException ex)
            {
                log.debug("JSON plan parsing failed for capability='{}'; trying YAML tree parsing", capabilityName, ex);
            }
        }

        return yamlObjectMapper.readTree(payload);
    }

    private void normalizePlanTree(JsonNode tree)
    {
        if (!(tree instanceof ObjectNode planNode))
        {
            return;
        }

        normalizePlanStatus(planNode);
        normalizeTaskStatuses(planNode.get("tasks"));
        planNode.remove("planId");
        planNode.put("planId", requireNonBlank(planIdSupplier.get(), "planId"));
    }

    private Map<String, Object> requireAcceptedAttempt(Map<String, Object> modelAttempt)
    {
        if (modelAttempt == null || modelAttempt.isEmpty())
        {
            throw new IllegalStateException("Accepted planning response must include model attempt context");
        }
        return Map.of(
                "attemptId", requireNonBlank((String) modelAttempt.get("attemptId"), "attemptId"),
                "retrySequenceId", requireNonBlank((String) modelAttempt.get("retrySequenceId"), "retrySequenceId"));
    }

    private String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }

    private void normalizePlanStatus(ObjectNode planNode)
    {
        JsonNode statusNode = planNode.get("status");
        if (statusNode == null || !statusNode.isTextual())
        {
            return;
        }

        String normalizedStatus = normalizePlanStatusValue(statusNode.asText());
        if (normalizedStatus != null)
        {
            planNode.put("status", normalizedStatus);
        }
    }

    private void normalizeTaskStatuses(@Nullable JsonNode tasksNode)
    {
        if (!(tasksNode instanceof ArrayNode taskArray))
        {
            return;
        }
        for (JsonNode taskNode : taskArray)
        {
            if (!(taskNode instanceof ObjectNode objectTaskNode))
            {
                continue;
            }
            JsonNode statusNode = objectTaskNode.get("status");
            if (statusNode == null || !statusNode.isTextual())
            {
                continue;
            }
            String normalizedStatus = normalizeTaskStatusValue(statusNode.asText());
            if (normalizedStatus != null)
            {
                objectTaskNode.put("status", normalizedStatus);
            }
        }
    }

    @Nullable
    private String normalizePlanStatusValue(String rawStatus)
    {
        String normalized = canonicalizeEnumToken(rawStatus);
        return switch (normalized)
        {
            case "VALID", "STALE", "INVALID" -> normalized;
            case "EXECUTED", "EXECUTING", "COMPLETED", "COMPLETE", "SUCCESS", "SUCCEEDED", "DONE", "READY", "PENDING", "IN_PROGRESS", "INPROGRESS", "ACTIVE", "RUNNING", "OPEN", "NEW", "CURRENT", "ONGOING", "STARTED" -> PlanStatus.VALID.name();
            case "FAILED", "FAILURE", "ERROR", "BLOCKED" -> PlanStatus.INVALID.name();
            default -> null;
        };
    }

    @Nullable
    private String normalizeTaskStatusValue(String rawStatus)
    {
        String normalized = canonicalizeEnumToken(rawStatus);
        return switch (normalized)
        {
            case "PENDING", "IN_PROGRESS", "COMPLETED", "BLOCKED" -> normalized;
            case "SUCCESS", "SUCCEEDED", "DONE", "COMPLETE", "EXECUTED" -> PlanTaskStatus.COMPLETED.name();
            case "RUNNING", "ACTIVE", "EXECUTING", "INPROGRESS" -> PlanTaskStatus.IN_PROGRESS.name();
            case "WAITING", "READY", "TODO", "NEW", "OPEN", "QUEUED", "NOT_STARTED" -> PlanTaskStatus.PENDING.name();
            case "FAILED", "FAILURE", "ERROR", "INVALID", "STALE" -> PlanTaskStatus.BLOCKED.name();
            default -> null;
        };
    }

    private String canonicalizeEnumToken(String rawStatus)
    {
        return rawStatus == null
                ? ""
                : rawStatus.trim().toUpperCase(Locale.ROOT).replace('-', '_').replace(' ', '_');
    }

    private String unwrapFencedBlock(String payload)
    {
        String safePayload = payload == null ? "" : payload.trim();
        if (safePayload.startsWith("```"))
        {
            int firstNewline = safePayload.indexOf('\n');
            int lastFence = safePayload.lastIndexOf("```");
            if (firstNewline >= 0 && lastFence > firstNewline)
            {
                return safePayload.substring(firstNewline + 1, lastFence).trim();
            }
        }

        if (safePayload.startsWith("---"))
        {
            safePayload = safePayload.substring(3).trim();
        }

        return safePayload;
    }

    private boolean looksLikeJson(String payload)
    {
        return payload.startsWith("{") || payload.startsWith("[");
    }

    private String preview(String payload)
    {
        if (payload == null || payload.isBlank())
        {
            return "<empty>";
        }

        String normalized = payload.replace('\n', ' ').replace('\r', ' ').trim();
        return normalized.length() <= 200 ? normalized : normalized.substring(0, 200);
    }

    private static ObjectMapper defaultObjectMapper()
    {
        return com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().planningJson();
    }

    private static ObjectMapper defaultYamlObjectMapper()
    {
        return com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().planningYaml();
    }

    private String stringifyPlan(ExecutionPlan plan)
    {
        return plan == null ? "" : plan.toString();
    }

    private record PlanningAttemptResult(ExecutionPlan plan,
            String prompt,
            String userMessage,
            Map<String, Object> modelAttempt)
    {
    }
}
