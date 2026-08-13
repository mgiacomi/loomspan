package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.internal.core.ExecutionPlan;
import com.lokiscale.loomspan.internal.core.PlanTask;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

class PlanQualityValidator
{
    private static final Set<String> REPORT_KEYWORDS = Set.of(
            "summary", "summarize", "report", "conclusion", "final", "answer", "respond", "response");
    private static final Set<String> EXTRACT_KEYWORDS = Set.of(
            "extract", "parse", "read", "capture", "collect", "ingest");
    private static final Set<String> LOOKUP_KEYWORDS = Set.of(
            "lookup", "look", "find", "search", "query", "match", "compare", "retrieve", "fetch", "check");

    public PlanQualityValidationResult validate(ExecutionPlan plan, List<BoundCapability> visibleTools)
    {
        Objects.requireNonNull(plan, "plan must not be null");

        List<BoundCapability> safeVisibleTools = visibleTools == null ? List.of()
                : visibleTools.stream()
                        .filter(Objects::nonNull)
                        .toList();

        Map<String, ToolSummary> toolSummaries = summarizeTools(safeVisibleTools);
        List<PlanQualityIssue> warnings = new ArrayList<>();
        List<PlanQualityIssue> errors = new ArrayList<>();

        warnings.addAll(findSuspiciousReportBindings(plan, toolSummaries));
        warnings.addAll(findDiverseRepeatedToolUsage(plan));
        classifySingleToolOveruse(plan, toolSummaries).ifPresent(issue -> (issue.severity() == PlanQualityIssue.Severity.ERROR ? errors : warnings).add(issue));

        return new PlanQualityValidationResult(warnings, errors);
    }

    private List<PlanQualityIssue> findSuspiciousReportBindings(ExecutionPlan plan, Map<String, ToolSummary> toolSummaries)
    {
        List<PlanQualityIssue> issues = new ArrayList<>();
        for (PlanTask task : plan.tasks())
        {
            TaskRole taskRole = inferTaskRole(task);
            if (taskRole != TaskRole.REPORT)
            {
                continue;
            }
            ToolSummary tool = toolSummaries.get(task.capabilityName());
            if (tool == null || !tool.isExtractionOrLookupOnly())
            {
                continue;
            }
            issues.add(new PlanQualityIssue(
                    "REPORT_TASK_TOOL_MISMATCH",
                    PlanQualityIssue.Severity.WARNING,
                    "Task '%s' looks like a report/conclusion step but is bound to '%s', which is described as %s."
                            .formatted(task.taskId(), task.capabilityName(), tool.conciseRoleDescription())));
        }
        return issues;
    }

    private List<PlanQualityIssue> findDiverseRepeatedToolUsage(ExecutionPlan plan)
    {
        List<PlanQualityIssue> issues = new ArrayList<>();
        Map<String, Set<TaskRole>> rolesByTool = new LinkedHashMap<>();
        for (PlanTask task : plan.tasks())
        {
            if (task.capabilityName() == null || task.capabilityName().isBlank())
            {
                continue;
            }
            TaskRole role = inferTaskRole(task);
            if (role == TaskRole.OTHER)
            {
                continue;
            }
            rolesByTool.computeIfAbsent(task.capabilityName(), ignored -> new LinkedHashSet<>()).add(role);
        }

        rolesByTool.forEach((toolName, roles) ->
        {
            if (roles.size() > 1)
            {
                issues.add(new PlanQualityIssue(
                        "REPEATED_TOOL_DIVERSE_INTENTS",
                        PlanQualityIssue.Severity.WARNING,
                        "Tool '%s' is reused for materially different task intents (%s)."
                                .formatted(toolName, roles.stream().map(Enum::name).toList())));
            }
        });
        return issues;
    }

    private Optional<PlanQualityIssue> classifySingleToolOveruse(ExecutionPlan plan, Map<String, ToolSummary> toolSummaries)
    {
        List<PlanTask> boundTasks = plan.tasks().stream()
                .filter(task -> task.capabilityName() != null && !task.capabilityName().isBlank())
                .toList();

        if (toolSummaries.size() < 2 || boundTasks.size() < 3)
        {
            return Optional.empty();
        }

        List<PlanTask> nonReportTasks = boundTasks.stream()
                .filter(task -> inferTaskRole(task) != TaskRole.REPORT)
                .toList();

        List<PlanTask> candidateTasks = nonReportTasks.isEmpty() ? boundTasks : nonReportTasks;
        Set<String> usedTools = candidateTasks.stream()
                .map(PlanTask::capabilityName)
                .collect(Collectors.toSet());

        if (usedTools.size() != 1)
        {
            return Optional.empty();
        }

        String dominantToolName = candidateTasks.getFirst().capabilityName();
        ToolSummary dominantTool = toolSummaries.get(dominantToolName);
        List<ToolSummary> unusedTools = toolSummaries.values().stream()
                .filter(tool -> !tool.name().equals(dominantToolName))
                .toList();

        if (dominantTool == null || unusedTools.isEmpty())
        {
            return Optional.empty();
        }

        Set<TaskRole> taskRoles = boundTasks.stream()
                .map(this::inferTaskRole)
                .filter(role -> role != TaskRole.OTHER)
                .collect(Collectors.toCollection(LinkedHashSet::new));

        boolean hasMixedTaskRoles = taskRoles.size() > 1;
        boolean hasDifferentAvailableRole = unusedTools.stream()
                .anyMatch(tool -> tool.primaryRole() != dominantTool.primaryRole());

        if (!hasDifferentAvailableRole)
        {
            return Optional.of(new PlanQualityIssue(
                    "SINGLE_TOOL_OVERUSE_WARNING",
                    PlanQualityIssue.Severity.WARNING,
                    "The plan routes every bound task through '%s' even though multiple tools are visible."
                            .formatted(dominantToolName)));
        }

        if (!hasMixedTaskRoles && !containsReportTask(boundTasks))
        {
            return Optional.empty();
        }

        String unusedToolNames = unusedTools.stream().map(ToolSummary::name).toList().toString();
        return Optional.of(new PlanQualityIssue(
                "SINGLE_TOOL_OVERUSE",
                PlanQualityIssue.Severity.ERROR,
                "The plan overuses '%s' across distinct tasks and skips other visible tools %s."
                        .formatted(dominantToolName, unusedToolNames)));
    }

    private boolean containsReportTask(List<PlanTask> tasks)
    {
        return tasks.stream().anyMatch(task -> inferTaskRole(task) == TaskRole.REPORT);
    }

    private Map<String, ToolSummary> summarizeTools(List<BoundCapability> visibleTools)
    {
        Map<String, ToolSummary> summaries = new LinkedHashMap<>();
        for (BoundCapability tool : visibleTools)
        {
            if (tool.name().isBlank())
            {
                continue;
            }
            summaries.put(tool.name(), ToolSummary.from(tool));
        }
        return summaries;
    }

    private TaskRole inferTaskRole(PlanTask task)
    {
        String text = normalizeText(task.title() + " " + nullSafe(task.intent()) + " " + String.join(" ", task.expectedOutputs()));
        if (containsAny(text, REPORT_KEYWORDS))
        {
            return TaskRole.REPORT;
        }
        if (containsAny(text, LOOKUP_KEYWORDS))
        {
            return TaskRole.LOOKUP;
        }
        if (containsAny(text, EXTRACT_KEYWORDS))
        {
            return TaskRole.EXTRACT;
        }
        return TaskRole.OTHER;
    }

    private static boolean containsAny(String text, Set<String> keywords)
    {
        for (String keyword : keywords)
        {
            if (text.contains(keyword))
            {
                return true;
            }
        }
        return false;
    }

    private static String normalizeText(String text)
    {
        return text == null ? "" : text.toLowerCase(Locale.ROOT);
    }

    private static String nullSafe(String text)
    {
        return text == null ? "" : text;
    }

    private enum TaskRole
    {
        EXTRACT,
        LOOKUP,
        REPORT,
        OTHER
    }

    private record ToolSummary(String name, TaskRole primaryRole, boolean extraction, boolean lookup, boolean report)
    {

        static ToolSummary from(BoundCapability definition)
        {
            String description = definition.description() == null || definition.description().isBlank()
                    ? "No description provided."
                    : definition.description();

            String normalized = normalizeText(definition.name() + " " + description);
            boolean extraction = containsAny(normalized, EXTRACT_KEYWORDS);
            boolean lookup = containsAny(normalized, LOOKUP_KEYWORDS);
            boolean report = containsAny(normalized, REPORT_KEYWORDS);
            TaskRole primaryRole = report ? TaskRole.REPORT : lookup ? TaskRole.LOOKUP : extraction ? TaskRole.EXTRACT : TaskRole.OTHER;
            return new ToolSummary(definition.name(), primaryRole, extraction, lookup, report);
        }

        boolean isExtractionOrLookupOnly()
        {
            return (extraction || lookup) && !report;
        }

        String conciseRoleDescription()
        {
            if (report)
            {
                return "report/synthesis";
            }
            if (lookup)
            {
                return extraction ? "extraction/lookup" : "lookup";
            }
            if (extraction)
            {
                return "extraction";
            }
            return "general-purpose";
        }
    }
}
