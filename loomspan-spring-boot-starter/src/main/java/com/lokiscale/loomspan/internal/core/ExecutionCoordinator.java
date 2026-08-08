package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.chat.SkillChatClientFactory;
import com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException;
import com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.runtime.tool.ToolCallbackFactory;
import com.lokiscale.loomspan.internal.runtime.tool.ToolSurfaceService;
import com.lokiscale.loomspan.internal.security.AccessGuard;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.tool.ToolCallback;
import org.springframework.core.io.Resource;
import org.springframework.lang.Nullable;
import org.springframework.security.core.Authentication;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;
import java.util.concurrent.CancellationException;

public class ExecutionCoordinator
{
    private final YamlSkillCatalog yamlSkillCatalog;
    private final CapabilityRegistry capabilityRegistry;
    private final SkillChatClientFactory skillChatClientFactory;
    private final ToolSurfaceService toolSurfaceService;
    private final ToolCallbackFactory toolCallbackFactory;
    private final MissionExecutionEngine missionExecutionEngine;
    private final MissionExecutionEngine stepLoopMissionExecutionEngine;
    private final ExecutionStateService executionStateService;
    private final AccessGuard accessGuard;

    public ExecutionCoordinator(YamlSkillCatalog yamlSkillCatalog,
            CapabilityRegistry capabilityRegistry,
            SkillChatClientFactory skillChatClientFactory,
            ToolSurfaceService toolSurfaceService,
            ToolCallbackFactory toolCallbackFactory,
            MissionExecutionEngine missionExecutionEngine,
            MissionExecutionEngine stepLoopMissionExecutionEngine,
            ExecutionStateService executionStateService,
            AccessGuard accessGuard)
    {
        this.yamlSkillCatalog = Objects.requireNonNull(yamlSkillCatalog, "yamlSkillCatalog must not be null");
        this.capabilityRegistry = Objects.requireNonNull(capabilityRegistry, "capabilityRegistry must not be null");
        this.skillChatClientFactory = Objects.requireNonNull(skillChatClientFactory, "skillChatClientFactory must not be null");
        this.toolSurfaceService = Objects.requireNonNull(toolSurfaceService, "toolSurfaceService must not be null");
        this.toolCallbackFactory = Objects.requireNonNull(toolCallbackFactory, "toolCallbackFactory must not be null");
        this.missionExecutionEngine = Objects.requireNonNull(missionExecutionEngine, "missionExecutionEngine must not be null");
        this.stepLoopMissionExecutionEngine = Objects.requireNonNull(stepLoopMissionExecutionEngine, "stepLoopMissionExecutionEngine must not be null");
        this.executionStateService = Objects.requireNonNull(executionStateService, "executionStateService must not be null");
        this.accessGuard = Objects.requireNonNull(accessGuard, "accessGuard must not be null");
    }

    public String execute(String skillName, String objective, LoomspanSession session, @Nullable Authentication authentication)
    {
        return execute(skillName, objective, null, session, authentication);
    }

    public String execute(String skillName,
            String objective,
            @Nullable Map<String, Object> missionInput,
            LoomspanSession session,
            @Nullable Authentication authentication)
    {
        Objects.requireNonNull(session, "session must not be null");
        requireNonBlank(objective, "objective");
        YamlSkillDefinition definition = requireYamlSkill(skillName);
        CapabilityMetadata rootCapability = requireCapability(skillName);
        boolean topLevelInvocation = session.getFramesSnapshot().isEmpty();
        if (topLevelInvocation && !rootCapability.name().equals(session.entrySkill()))
        {
            throw new IllegalArgumentException("Top-level capability name does not match session entry skill");
        }

        if (authentication != null || session.getFramesSnapshot().isEmpty())
        {
            session.setAuthentication(authentication);
        }

        accessGuard.checkAccess(rootCapability, session, authentication);
        executionStateService.clearPlan(session);

        // Every YAML skill run gets a fresh successful-direct-skill ledger; nested invocations rely on
        // CapabilityExecutionRouter to snapshot and restore the parent's successful skills afterward.
        executionStateService.clearSuccessfulSkills(session);
        LinkedHashMap<String, Object> frameParameters = new LinkedHashMap<>();
        frameParameters.put("objective", objective);

        if (missionInput != null && !missionInput.isEmpty())
        {
            frameParameters.put("missionInput", traceSafeMissionInput(definition, missionInput));
        }

        ExecutionFrame frame = executionStateService.openMissionFrame(session, rootCapability.name(), Map.copyOf(frameParameters));
        Throwable failure = null;
        String terminalFailureId = null;

        try
        {
            return LoomspanSessionHolder.callWithSession(session, () ->
            {
                boolean stepExecutionEnabled = definition.planningModeExplicitlyEnabled();
                MissionExecutionEngine engine = stepExecutionEnabled ? stepLoopMissionExecutionEngine : missionExecutionEngine;
                ChatClient chatClient = stepExecutionEnabled
                        ? skillChatClientFactory.createForStepExecution(definition)
                        : skillChatClientFactory.create(definition);

                List<ToolCallback> visibleTools = toolCallbackFactory.createToolCallbacks(
                        session,
                        definition,
                        toolSurfaceService.visibleToolsFor(skillName, session, authentication),
                        authentication);

                return engine.executeMission(
                        session,
                        definition,
                        objective,
                        missionInput,
                        chatClient,
                        visibleTools,
                        definition.planningModeExplicitlyEnabled(),
                        authentication);
            });
        }
        catch (RuntimeException | Error ex)
        {
            failure = ex;
            terminalFailureId = UUID.randomUUID().toString();
            executionStateService.logError(session, terminalFailureId, errorPayload(skillName, objective, ex));
            session.markTraceErrored();
            throw ex;
        }
        finally
        {
            RuntimeException cleanupFailure = null;
            try
            {
                executionStateService.closeFrame(session, frame, closeMetadata(failure, terminalFailureId));
            }
            catch (RuntimeException ex)
            {
                cleanupFailure = ex;
                if (terminalFailureId == null)
                {
                    terminalFailureId = UUID.randomUUID().toString();
                    try
                    {
                        executionStateService.logError(
                                session,
                                terminalFailureId,
                                cleanupErrorPayload(skillName, objective, ex));
                    }
                    catch (RuntimeException errorRecordingFailure)
                    {
                        ex.addSuppressed(errorRecordingFailure);
                    }
                }
            }
            if (topLevelInvocation)
            {
                try
                {
                    TraceOutcome outcome = terminalOutcome(failure, cleanupFailure);
                    executionStateService.finalizeTrace(session, new TraceCompletion(
                            outcome,
                            session.getSessionUsage().orElse(
                                    com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.empty()),
                            terminalFailureId,
                            Map.of(
                                    "skillName", skillName,
                                    "objective", objective,
                                    "remainingFrames", session.getFramesSnapshot().size())));
                }
                catch (RuntimeException ex)
                {
                    if (cleanupFailure == null)
                    {
                        cleanupFailure = ex;
                    }
                    else
                    {
                        cleanupFailure.addSuppressed(ex);
                    }
                }
            }
            if (cleanupFailure != null)
            {
                if (failure != null)
                {
                    failure.addSuppressed(cleanupFailure);
                }
                else
                {
                    throw cleanupFailure;
                }
            }
        }
    }

    private TraceOutcome terminalOutcome(@Nullable Throwable failure, @Nullable Throwable cleanupFailure)
    {
        if (failure == null && cleanupFailure == null)
        {
            return TraceOutcome.SUCCEEDED;
        }
        return Thread.currentThread().isInterrupted() || isCancellation(failure) || isCancellation(cleanupFailure)
                ? TraceOutcome.ABORTED
                : TraceOutcome.FAILED;
    }

    private boolean isCancellation(@Nullable Throwable failure)
    {
        Throwable current = failure;
        while (current != null)
        {
            if (current instanceof LoomspanMissionTimeoutException
                    || current instanceof CancellationException
                    || current instanceof InterruptedException)
            {
                return true;
            }
            current = current.getCause();
        }
        return false;
    }

    private Map<String, Object> closeMetadata(@Nullable Throwable failure, @Nullable String failureId)
    {
        LinkedHashMap<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("status", failure == null
                ? "completed"
                : (Thread.currentThread().isInterrupted() || isCancellation(failure) ? "aborted" : "failed"));
        if (failure != null)
        {
            metadata.put("failureId", Objects.requireNonNull(failureId, "failureId must not be null"));
            TraceFailureMetadata.addTo(metadata, failure, "Mission execution failed");
        }
        return metadata;
    }

    private Map<String, Object> cleanupErrorPayload(String skillName, String objective, Throwable failure)
    {
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("skillName", skillName);
        payload.put("objective", objective);
        TraceFailureMetadata.addTo(payload, failure, "Mission finalization failed");
        return Map.copyOf(payload);
    }

    private Map<String, Object> errorPayload(String skillName, String objective, Throwable failure)
    {
        LinkedHashMap<String, Object> payload = new LinkedHashMap<>();
        payload.put("skillName", skillName);
        payload.put("objective", objective);
        TraceFailureMetadata.addTo(payload, failure, "Mission execution failed");
        return payload;
    }

    private Object traceSafeMissionInput(YamlSkillDefinition definition, Object value)
    {
        if (!definition.hasDeclaredInputSchema())
        {
            return value;
        }
        return traceSafeNode(definition.inputSchema(), value, "");
    }

    private Object traceSafeNode(YamlSkillManifest.InputSchemaManifest schema, Object value, String path)
    {
        if (schema == null)
        {
            return value;
        }
        if ("attachment".equals(schema.getType()))
        {
            return attachmentPlaceholder(value, path, schema);
        }
        if ("object".equals(schema.getType()) && value instanceof Map<?, ?> map)
        {
            LinkedHashMap<String, Object> safe = new LinkedHashMap<>();
            map.forEach((key, childValue) ->
            {
                String childName = String.valueOf(key);
                safe.put(childName, traceSafeNode(
                        schema.getProperties().get(childName),
                        childValue,
                        join(path, childName)));
            });
            return Map.copyOf(safe);
        }
        if ("array".equals(schema.getType()) && value instanceof List<?> list && schema.getItems() != null)
        {
            List<Object> safe = new ArrayList<>(list.size());
            for (int index = 0; index < list.size(); index++)
            {
                safe.add(traceSafeNode(schema.getItems(), list.get(index), path + "[" + index + "]"));
            }
            return List.copyOf(safe);
        }
        return value;
    }

    private Map<String, Object> attachmentPlaceholder(Object value,
            String path,
            YamlSkillManifest.InputSchemaManifest schema)
    {
        LinkedHashMap<String, Object> placeholder = new LinkedHashMap<>();
        placeholder.put("attachment", true);
        placeholder.put("fieldPath", path);
        placeholder.put("mediaType", schema.getMediaType());
        placeholder.put("allowedContentTypes", schema.getAllowedContentTypes());
        if (value instanceof String text && text.matches("^ref://\\S+$"))
        {
            placeholder.put("source", text);
        }
        else if (value instanceof Resource resource)
        {
            placeholder.put("source", "resource");
            placeholder.put("name", resource.getFilename());
        }
        else
        {
            placeholder.put("source", "redacted");
        }
        return Map.copyOf(placeholder);
    }

    private String join(String parent, String child)
    {
        return parent == null || parent.isBlank() ? child : parent + "." + child;
    }

    private YamlSkillDefinition requireYamlSkill(String skillName)
    {
        YamlSkillDefinition definition = yamlSkillCatalog.getSkill(skillName);
        if (definition == null)
        {
            throw new IllegalArgumentException("Unknown YAML skill '" + skillName + "'");
        }
        return definition;
    }

    private CapabilityMetadata requireCapability(String skillName)
    {
        CapabilityMetadata capability = capabilityRegistry.getCapability(skillName);
        if (capability == null)
        {
            throw new IllegalArgumentException("Unknown capability '" + skillName + "'");
        }
        if (capability.kind() != CapabilityKind.YAML_SKILL || !skillName.equals(capability.name()))
        {
            throw new IllegalStateException("CapabilityRegistry returned inconsistent public YAML metadata for '"
                    + skillName + "'");
        }
        return capability;
    }

    private static String requireNonBlank(String value, String fieldName)
    {
        Objects.requireNonNull(value, fieldName + " must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException(fieldName + " must not be blank");
        }
        return value;
    }
}
