package com.lokiscale.loomspan.internal.runtime.step;

import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.core.PlanTaskStatus;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationResult;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Validates a proposed {@link StepAction} against the current plan state before any side effects occur.
 */
final class StepActionValidator
{
    private static final Set<String> PLACEHOLDER_SENTINELS = Set.of(
            "<string>",
            "<value>",
            "<number>",
            "<boolean>",
            "<key>",
            "<canonical mission input>");

    private static final SkillInputValidator INPUT_VALIDATOR = new SkillInputValidator();

    private StepActionValidator()
    {
    }

    public static StepValidationResult validate(StepAction action,
            ExecutionPlan plan,
            List<BoundCapability> visibleTools,
            boolean strictCompletion)
    {
        Objects.requireNonNull(plan, "plan must not be null");

        if (action == null)
        {
            return StepValidationResult.rejected("Model returned null action.");
        }

        if (action.stepAction() == null)
        {
            return StepValidationResult.rejected("Step action type is null. Expected CALL_TOOL or FINAL_RESPONSE.");
        }

        return switch (action.stepAction())
        {
            case CALL_TOOL -> validateCallTool(action, plan, visibleTools);
            case FINAL_RESPONSE -> validateFinalResponse(action, plan, strictCompletion);
        };
    }

    private static StepValidationResult validateCallTool(StepAction action, ExecutionPlan plan, List<BoundCapability> visibleTools)
    {
        boolean allTasksCompleted = plan.tasks().stream().allMatch(task -> task.status() == PlanTaskStatus.COMPLETED);

        if (allTasksCompleted)
        {
            return StepValidationResult.rejected(
                    "All required plan tasks are already completed. Return FINAL_RESPONSE instead of CALL_TOOL.");
        }

        if (action.taskId() == null || action.taskId().isBlank())
        {
            return StepValidationResult.rejected("CALL_TOOL action requires a taskId.");
        }

        if (action.toolName() == null || action.toolName().isBlank())
        {
            return StepValidationResult.rejected("CALL_TOOL action requires a toolName.");
        }

        Optional<PlanTask> targetTask = plan.findTask(action.taskId());
        if (targetTask.isEmpty())
        {
            return StepValidationResult.rejected("Task '%s' does not exist in the plan.".formatted(action.taskId()));
        }

        PlanTask task = targetTask.get();
        java.util.Map<String, PlanTask> tasksById = plan.tasks().stream()
                .collect(Collectors.toMap(PlanTask::taskId, current -> current, (left, right) -> left));

        if (!task.isReady(tasksById))
        {
            return StepValidationResult.rejected(
                    "Task '%s' is not ready (status=%s). Only PENDING tasks with completed dependencies may be executed."
                            .formatted(action.taskId(), task.status()));
        }

        Set<String> validToolNames = visibleTools.stream()
                .filter(tool -> tool != null)
                .map(tool -> tool.name())
                .collect(Collectors.toSet());

        if (!validToolNames.contains(action.toolName()))
        {
            return StepValidationResult.rejected(
                    "Tool '%s' is not in the available tools: %s".formatted(action.toolName(), validToolNames));
        }

        if (task.capabilityName() == null || task.capabilityName().isBlank())
        {
            return StepValidationResult.rejected(
                    "Task '%s' does not declare an allowed tool capability.".formatted(action.taskId()));
        }

        if (!task.capabilityName().equals(action.toolName()))
        {
            return StepValidationResult.rejected(
                    "Task '%s' expects tool '%s' but model proposed '%s'."
                            .formatted(action.taskId(), task.capabilityName(), action.toolName()));
        }

        StepValidationResult schemaValidation = validateRequiredToolArguments(action, visibleTools);
        if (!schemaValidation.valid())
        {
            return schemaValidation;
        }

        return StepValidationResult.ok();
    }

    private static StepValidationResult validateRequiredToolArguments(StepAction action, List<BoundCapability> visibleTools)
    {
        Optional<BoundCapability> matchingTool = visibleTools.stream()
                .filter(tool -> tool != null)
                .filter(tool -> action.toolName().equals(tool.name()))
                .findFirst();

        if (matchingTool.isEmpty())
        {
            return StepValidationResult.ok();
        }

        SkillInputContract contract = matchingTool.get().metadata().inputContract();
        if (contract.isGeneric())
        {
            return StepValidationResult.ok();
        }

        Map<String, Object> arguments = action.toolArguments() == null ? Map.of() : action.toolArguments();
        SkillInputValidationResult validation = INPUT_VALIDATOR.validate(arguments, contract);
        if (!validation.valid())
        {
            String detail = validation.issues().stream()
                    .map(issue -> (issue.path() == null || issue.path().isBlank() ? "<root>" : issue.path())
                            + " [" + issue.code() + "]: " + issue.message())
                    .reduce((left, right) -> left + "; " + right)
                    .orElse("Invalid tool arguments.");
            return StepValidationResult.rejected(
                    "Tool '%s' arguments failed validation: %s".formatted(action.toolName(), detail));
        }
        List<String> placeholderPaths = new ArrayList<>();
        collectPlaceholderPaths(validation.normalizedInput(), "", placeholderPaths);
        if (!placeholderPaths.isEmpty())
        {
            return StepValidationResult.rejected(
                    "Tool '%s' arguments contain unresolved placeholder values at: %s"
                            .formatted(action.toolName(), String.join(", ", placeholderPaths)));
        }
        return StepValidationResult.ok();
    }

    private static void collectPlaceholderPaths(Object value, String path, List<String> placeholderPaths)
    {
        if (value instanceof Map<?, ?> mapValue)
        {
            mapValue.forEach((key, nestedValue) -> collectPlaceholderPaths(nestedValue, joinPath(path, String.valueOf(key)), placeholderPaths));
            return;
        }
        if (value instanceof List<?> listValue)
        {
            for (int index = 0; index < listValue.size(); index++)
            {
                collectPlaceholderPaths(listValue.get(index), path + "[" + index + "]", placeholderPaths);
            }
            return;
        }
        if (value instanceof String text && looksLikePlaceholder(text))
        {
            placeholderPaths.add(path.isBlank() ? "<root>" : path);
        }
    }

    private static boolean looksLikePlaceholder(String value)
    {
        String normalized = value == null ? "" : value.trim();
        if (normalized.isBlank())
        {
            return false;
        }
        if (PLACEHOLDER_SENTINELS.contains(normalized))
        {
            return true;
        }

        return normalized.startsWith("<")
                && normalized.endsWith(">")
                && normalized.length() > 2
                && normalized.substring(1, normalized.length() - 1).chars().allMatch(character -> Character.isLetterOrDigit(character)
                        || Character.isWhitespace(character)
                        || character == '_'
                        || character == '-');
    }

    private static String joinPath(String parent, String child)
    {
        return parent == null || parent.isBlank() ? child : parent + "." + child;
    }

    private static StepValidationResult validateFinalResponse(StepAction action,
            ExecutionPlan plan,
            boolean strictCompletion)
    {
        if (action.finalResponse() == null || action.finalResponse().isNull())
        {
            return StepValidationResult.rejected("FINAL_RESPONSE action requires a non-empty finalResponse.");
        }

        if (strictCompletion)
        {
            List<PlanTask> incompleteTasks = plan.tasks().stream()
                    .filter(task -> task.status() != PlanTaskStatus.COMPLETED)
                    .toList();

            if (!incompleteTasks.isEmpty())
            {
                String incompleteIds = incompleteTasks.stream()
                        .map(task -> task.taskId() + "(" + task.status() + ")")
                        .collect(Collectors.joining(", "));
                return StepValidationResult.rejected(
                        "Cannot finalize: %d task(s) remain incomplete: [%s]."
                                .formatted(incompleteTasks.size(), incompleteIds));
            }
        }

        return StepValidationResult.ok();
    }
}
