package com.lokiscale.loomspan.internal.runtime.step;

import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.MissionInputMessageFormatter;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputPromptRenderer;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputSchemaNode;
import com.lokiscale.loomspan.internal.skill.YamlSkillManifest;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;
import org.springframework.lang.Nullable;

import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.stream.Collectors;

/**
 * Builds a concise, task-focused system prompt for each iteration of the plan-step execution loop.
 */
final class StepPromptBuilder
{
    private static final int MAX_LAST_RESULT_CHARS = 1000;
    private static final SkillInputPromptRenderer INPUT_PROMPT_RENDERER = new SkillInputPromptRenderer();

    private StepPromptBuilder()
    {
    }

    public static String buildStepPrompt(ExecutionPlan plan,
            String objective,
            int stepNumber,
            @Nullable String lastToolResult,
            @Nullable String executionSummary,
            List<BoundCapability> visibleTools,
            boolean finalResponseOnly,
            @Nullable YamlSkillManifest.OutputSchemaManifest outputSchema)
    {
        return buildStepPrompt(
                plan,
                objective,
                null,
                stepNumber,
                lastToolResult,
                executionSummary,
                visibleTools,
                finalResponseOnly,
                false,
                outputSchema);
    }

    public static String buildStepPrompt(ExecutionPlan plan,
            String objective,
            @Nullable Map<String, Object> missionInput,
            int stepNumber,
            @Nullable String lastToolResult,
            @Nullable String executionSummary,
            List<BoundCapability> visibleTools,
            boolean finalResponseOnly,
            @Nullable YamlSkillManifest.OutputSchemaManifest outputSchema)
    {
        return buildStepPrompt(
                plan,
                objective,
                missionInput,
                stepNumber,
                lastToolResult,
                executionSummary,
                visibleTools,
                finalResponseOnly,
                false,
                outputSchema);
    }

    public static String buildStepPrompt(ExecutionPlan plan,
            String objective,
            @Nullable Map<String, Object> missionInput,
            int stepNumber,
            @Nullable String lastToolResult,
            @Nullable String executionSummary,
            List<BoundCapability> visibleTools,
            boolean finalResponseOnly,
            boolean forceVerboseToolArgumentGuidance,
            @Nullable YamlSkillManifest.OutputSchemaManifest outputSchema)
    {
        Objects.requireNonNull(plan, "plan must not be null");
        Objects.requireNonNull(objective, "objective must not be null");

        String readyTaskLines = formatTasks(plan.readyTasks(), plan);
        String waitingTaskLines = formatWaitingTasks(plan);
        String blockedTaskLines = formatTasksByStatus(plan, PlanTaskStatus.BLOCKED);
        String completedTaskLines = formatTasksByStatus(plan, PlanTaskStatus.COMPLETED);
        String activeTaskLine = plan.activeTask()
                .map(task -> task.taskId() + ": " + task.title())
                .orElse("(none)");

        String toolNameList = (visibleTools == null || visibleTools.isEmpty())
                ? "(none)"
                : visibleTools.stream()
                        .filter(t -> t != null)
                        .map(t -> "- " + t.name())
                        .reduce((a, b) -> a + "\n" + b)
                        .orElse("(none)");

        String currentStepInstructions = formatCurrentStepInstructions(plan.readyTasks());
        String missionContext = MissionInputMessageFormatter.buildMissionContext(objective, null, plan.capabilityName());

        StringBuilder sb = new StringBuilder();
        sb.append("""
                You are executing a planned mission step by step.
                Mission context:
                %s
                Step: %d
                Plan: %s (status: %s)
                """.formatted(missionContext, stepNumber, plan.planId(), plan.status()));

        sb.append("\nActive task: ").append(activeTaskLine);
        sb.append("\n\n--- COMPLETED TASKS ---\n").append(completedTaskLines);
        sb.append("\n\n--- READY TASKS (you should work on one of these) ---\n").append(readyTaskLines);
        sb.append("\n\n--- PENDING TASKS WAITING ON DEPENDENCIES ---\n").append(waitingTaskLines);
        sb.append("\n\n--- BLOCKED TASKS (failed or cannot continue) ---\n").append(blockedTaskLines);

        if (currentStepInstructions != null)
        {
            sb.append("\n\n--- CURRENT EXECUTABLE TASK ---\n").append(currentStepInstructions);
        }

        String toolArgumentGuidance = formatToolArgumentGuidance(plan.readyTasks(), visibleTools, forceVerboseToolArgumentGuidance);
        if (toolArgumentGuidance != null)
        {
            sb.append("\n\n--- TOOL ARGUMENT SHAPE ---\n").append(toolArgumentGuidance);
        }

        if (executionSummary != null && !executionSummary.isBlank())
        {
            sb.append("\n\n--- EXECUTION SUMMARY ---\n").append(executionSummary);
        }

        if (lastToolResult != null && !lastToolResult.isBlank())
        {
            String trimmedResult = lastToolResult.length() > MAX_LAST_RESULT_CHARS
                    ? lastToolResult.substring(0, MAX_LAST_RESULT_CHARS) + "... (truncated)"
                    : lastToolResult;
            sb.append("\n\n--- LAST TOOL RESULT ---\n").append(trimmedResult);
        }

        sb.append("\n\n--- AVAILABLE TOOLS ---\n").append(toolNameList);

        if (finalResponseOnly)
        {
            sb.append("""


                    --- YOUR TASK ---
                    All required plan tasks are already COMPLETE.
                    Return ONLY valid JSON - no markdown, no explanation, no code fences.
                    You must return a FINAL_RESPONSE action. CALL_TOOL is not allowed anymore.

                    {
                      "stepAction": "FINAL_RESPONSE",
                      "finalResponse": { <your complete response to the mission objective> }
                    }
                    """);

            appendOutputSchemaGuidance(sb, outputSchema);
            sb.append("""

                    Rules:
                    - Do NOT call any tool.
                    - finalResponse must be a raw JSON object matching the required schema, not a string containing escaped JSON.
                    - Return raw JSON only.
                    """);
        }
        else
        {
            String expectedTaskJson = """
                    {
                      "stepAction": "CALL_TOOL",
                      "taskId": "<taskId of the ready task>",
                      "toolName": "<exact tool name from the list above>",
                      "toolArguments": { <arguments for this tool> }
                    }""";

            List<PlanTask> readyTasks = plan.readyTasks();
            if (readyTasks != null && readyTasks.size() == 1)
            {
                PlanTask singleTask = readyTasks.getFirst();
                String toolName = singleTask.capabilityName() == null ? "unspecified" : singleTask.capabilityName();
                expectedTaskJson = """
                        {
                          "stepAction": "CALL_TOOL",
                          "taskId": "%s",
                          "toolName": "%s",
                          "toolArguments": { <arguments for this tool> }
                        }""".formatted(singleTask.taskId(), toolName);
            }

            sb.append("""


                    --- YOUR TASK ---
                    Return ONLY valid JSON - no markdown, no explanation, no code fences.

                    You must call a tool for one of the READY tasks:
                    """).append(expectedTaskJson).append("""

                    Rules:
                    - You MUST pick a task from the READY list. Do NOT pick waiting, blocked, or completed tasks.
                    - The ready task is already bound to a specific tool. Do not use the mission skill name as toolName.
                    - Use the exact toolName and taskId values shown above.
                    - Return raw JSON only.
                    """);
        }

        return sb.toString();
    }

    public static String buildStepUserMessage(ExecutionPlan plan, String objective)
    {
        return buildStepUserMessage(plan, objective, null);
    }

    public static String buildStepUserMessage(ExecutionPlan plan, String objective, @Nullable Map<String, Object> missionInput)
    {
        Objects.requireNonNull(plan, "plan must not be null");
        Objects.requireNonNull(objective, "objective must not be null");
        return MissionInputMessageFormatter.buildUserMessage(
                MissionInputMessageFormatter.buildMissionContext(objective, null, plan.capabilityName()),
                missionInput);
    }

    private static String formatTasks(List<PlanTask> tasks, ExecutionPlan plan)
    {
        if (tasks.isEmpty())
        {
            return "  (none)";
        }

        return tasks.stream()
                .sorted(Comparator.comparingInt(plan.tasks()::indexOf))
                .map(task -> "  - [%s] %s: %s (tool: %s)%s".formatted(
                        task.status(),
                        task.taskId(),
                        task.title(),
                        task.capabilityName() == null ? "unspecified" : task.capabilityName(),
                        task.note() == null ? "" : " - " + task.note()))
                .reduce((a, b) -> a + "\n" + b)
                .orElse("  (none)");
    }

    private static String formatTasksByStatus(ExecutionPlan plan, PlanTaskStatus status)
    {
        List<PlanTask> matching = plan.tasks().stream()
                .filter(task -> task.status() == status)
                .toList();
        if (matching.isEmpty())
        {
            return "  (none)";
        }
        return matching.stream()
                .map(task -> "  - %s: %s%s".formatted(
                        task.taskId(),
                        task.title(),
                        task.note() == null ? "" : " - " + task.note()))
                .reduce((a, b) -> a + "\n" + b)
                .orElse("  (none)");
    }

    private static String formatWaitingTasks(ExecutionPlan plan)
    {
        Map<String, PlanTask> tasksById = plan.tasks().stream()
                .collect(Collectors.toMap(PlanTask::taskId, task -> task, (left, right) -> left));

        List<PlanTask> waiting = plan.tasks().stream()
                .filter(task -> task.status() == PlanTaskStatus.PENDING)
                .filter(task -> !task.isReady(tasksById))
                .toList();

        if (waiting.isEmpty())
        {
            return "  (none)";
        }

        return waiting.stream()
                .map(task -> "  - %s: %s (waiting on: %s)%s".formatted(
                        task.taskId(),
                        task.title(),
                        task.dependsOn().isEmpty() ? "unknown" : String.join(", ", task.dependsOn()),
                        task.note() == null ? "" : " - " + task.note()))
                .reduce((a, b) -> a + "\n" + b)
                .orElse("  (none)");
    }

    private static void appendOutputSchemaGuidance(StringBuilder sb,
            @Nullable YamlSkillManifest.OutputSchemaManifest outputSchema)
    {
        if (outputSchema == null)
        {
            return;
        }
        sb.append("\n\n--- REQUIRED FINAL RESPONSE SHAPE ---\n");
        sb.append(renderSchemaExample(outputSchema, 0));
        if (outputSchema.getRequired() != null && !outputSchema.getRequired().isEmpty())
        {
            sb.append("\nRequired top-level fields: ")
                    .append(String.join(", ", outputSchema.getRequired()))
                    .append("\n");
        }
        List<String> nullableFields = nullableFieldPaths(outputSchema);
        if (!nullableFields.isEmpty())
        {
            sb.append("Nullable fields may be JSON null: ")
                    .append(String.join(", ", nullableFields))
                    .append("\n");
        }
        sb.append("Do not add fields that are not in this schema.\n");
    }

    private static String renderSchemaExample(YamlSkillManifest.OutputSchemaManifest schema, int depth)
    {
        if (Boolean.TRUE.equals(schema.getNullable()))
        {
            return "null";
        }
        String indent = "  ".repeat(depth);
        if ("object".equals(schema.getType()))
        {
            StringBuilder sb = new StringBuilder();
            sb.append("{\n");
            List<String> propertyNames = schema.getProperties().keySet().stream().sorted().toList();
            for (int i = 0; i < propertyNames.size(); i++)
            {
                String propertyName = propertyNames.get(i);
                YamlSkillManifest.OutputSchemaManifest child = schema.getProperties().get(propertyName);
                sb.append(indent)
                        .append("  \"")
                        .append(propertyName)
                        .append("\": ")
                        .append(renderSchemaExample(child, depth + 1));
                if (i < propertyNames.size() - 1)
                {
                    sb.append(",");
                }
                sb.append("\n");
            }
            sb.append(indent).append("}");
            return sb.toString();
        }
        if ("array".equals(schema.getType()))
        {
            String items = schema.getItems() == null ? "\"<value>\"" : renderSchemaExample(schema.getItems(), depth + 1);
            return "[ " + items + " ]";
        }
        if (schema.getEnumValues() != null && !schema.getEnumValues().isEmpty())
        {
            return "\"<one of: " + String.join(", ", schema.getEnumValues()) + ">\"";
        }
        return switch (schema.getType() == null ? "" : schema.getType())
        {
            case "string" -> "\"<string>\"";
            case "number", "integer" -> "<number>";
            case "boolean" -> "<boolean>";
            default -> "\"<value>\"";
        };
    }

    private static List<String> nullableFieldPaths(YamlSkillManifest.OutputSchemaManifest schema)
    {
        return nullableFieldPaths(schema, "").stream().sorted().toList();
    }

    private static List<String> nullableFieldPaths(YamlSkillManifest.OutputSchemaManifest schema, String path)
    {
        if (schema == null)
        {
            return List.of();
        }
        java.util.ArrayList<String> paths = new java.util.ArrayList<>();
        if (!path.isBlank() && Boolean.TRUE.equals(schema.getNullable()))
        {
            paths.add(path);
        }
        if ("object".equals(schema.getType()))
        {
            schema.getProperties().forEach((propertyName, child) ->
                    paths.addAll(nullableFieldPaths(child, path.isBlank() ? propertyName : path + "." + propertyName)));
        }
        if ("array".equals(schema.getType()) && schema.getItems() != null)
        {
            paths.addAll(nullableFieldPaths(schema.getItems(), path.isBlank() ? "[]" : path + "[]"));
        }
        return paths;
    }

    @Nullable
    private static String formatCurrentStepInstructions(List<PlanTask> readyTasks)
    {
        if (readyTasks == null || readyTasks.isEmpty())
        {
            return null;
        }

        if (readyTasks.size() == 1)
        {
            PlanTask task = readyTasks.getFirst();
            String toolName = task.capabilityName() == null ? "unspecified" : task.capabilityName();
            return """
                    Use taskId %s.
                    The only valid toolName for this step is %s.
                    Do not call the parent mission skill or invent a new tool name.
                    """.formatted(task.taskId(), toolName);
        }

        return readyTasks.stream()
                .map(task -> "  - taskId %s must use toolName %s".formatted(
                        task.taskId(),
                        task.capabilityName() == null ? "unspecified" : task.capabilityName()))
                .reduce((a, b) -> a + "\n" + b)
                .orElse(null);
    }

    @Nullable
    private static String formatToolArgumentGuidance(List<PlanTask> readyTasks,
            List<BoundCapability> visibleTools,
            boolean forceVerboseToolArgumentGuidance)
    {
        if (readyTasks == null || readyTasks.isEmpty() || visibleTools == null || visibleTools.isEmpty())
        {
            return null;
        }

        return readyTasks.stream()
                .map(task -> formatToolArgumentGuidance(task, visibleTools, forceVerboseToolArgumentGuidance))
                .filter(Objects::nonNull)
                .reduce((left, right) -> left + "\n\n" + right)
                .orElse(null);
    }

    @Nullable
    private static String formatToolArgumentGuidance(PlanTask task,
            List<BoundCapability> visibleTools,
            boolean forceVerboseToolArgumentGuidance)
    {
        if (task.capabilityName() == null || task.capabilityName().isBlank())
        {
            return null;
        }

        BoundCapability tool = visibleTools.stream()
                .filter(candidate -> candidate != null)
                .filter(candidate -> task.capabilityName().equals(candidate.name()))
                .findFirst()
                .orElse(null);

        if (tool == null)
        {
            return null;
        }

        SkillInputContract contract = tool.metadata().inputContract();
        if (contract.isGeneric())
        {
            return null;
        }

        SkillInputPromptRenderer.DetailLevel detailLevel = forceVerboseToolArgumentGuidance || useVerboseDetail(contract.schema())
                ? SkillInputPromptRenderer.DetailLevel.VERBOSE
                : SkillInputPromptRenderer.DetailLevel.COMPACT;

        return """
                Task %s / tool %s:
                %s
                """.formatted(task.taskId(), task.capabilityName(), INPUT_PROMPT_RENDERER.renderToolArgumentsExample(contract, detailLevel));
    }

    private static boolean useVerboseDetail(SkillInputSchemaNode schema)
    {
        return maxDepth(schema, 1) > 2 || countProperties(schema) > 6;
    }

    private static int maxDepth(SkillInputSchemaNode schema, int depth)
    {
        if (schema == null)
        {
            return depth;
        }
        if (schema.isArray())
        {
            return maxDepth(schema.items(), depth + 1);
        }
        if (!schema.isObject())
        {
            return depth;
        }

        int propertyDepth = schema.properties().values().stream()
                .mapToInt(child -> maxDepth(child, depth + 1))
                .max()
                .orElse(depth);

        int additionalDepth = schema.additionalPropertiesSchema() == null
                ? depth
                : maxDepth(schema.additionalPropertiesSchema(), depth + 1);

        return Math.max(propertyDepth, additionalDepth);
    }

    private static int countProperties(SkillInputSchemaNode schema)
    {
        if (schema == null || !schema.isObject())
        {
            return 0;
        }

        return schema.properties().size()
                + schema.properties().values().stream().mapToInt(StepPromptBuilder::countProperties).sum()
                + countProperties(schema.additionalPropertiesSchema());
    }
}
