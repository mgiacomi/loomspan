package com.lokiscale.loomspan.internal.runtime.observation;

import tools.jackson.databind.JsonNode;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceOutcome;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import com.lokiscale.loomspan.internal.runtime.usage.UsagePrecision;

import java.util.ArrayList;
import java.util.EnumSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

public class LiveActivityProjector
{
    private static final Set<TraceRecordType> VISIBLE = EnumSet.of(
            TraceRecordType.TRACE_STARTED,
            TraceRecordType.MODEL_REQUEST_SENT,
            TraceRecordType.MODEL_RESPONSE_RECEIVED,
            TraceRecordType.MODEL_ATTEMPT_FAILED,
            TraceRecordType.PLAN_CREATED,
            TraceRecordType.PLAN_UPDATED,
            TraceRecordType.PLAN_VALIDATION_FAILED,
            TraceRecordType.PLAN_RETRY_REQUESTED,
            TraceRecordType.TOOL_CALL_STARTED,
            TraceRecordType.TOOL_CALL_COMPLETED,
            TraceRecordType.TOOL_CALL_FAILED,
            TraceRecordType.STEP_STARTED,
            TraceRecordType.STEP_ACTION_REJECTED,
            TraceRecordType.STEP_COMPLETED,
            TraceRecordType.ERROR_RECORDED,
            TraceRecordType.TRACE_COMPLETED);

    private static final List<String> DETAIL_KEYS = List.of(
            "skillName", "segment", "retrySequenceId", "attemptId", "attemptNumber",
            "capabilityName", "linkedTaskId", "unplanned", "planId", "stepNumber",
            "stepAction", "retry", "reason", "exhausted", "failureId", "classification",
            "exceptionType", "message", "outcome", "terminalFailureId", "providerAttemptNumber",
            "attemptReason", "failureClassification", "failureCategory", "retryDecision",
            "retryDelayMillis", "retryDelaySource");

    Projection project(ExecutionProjectionState state, TraceRecord record)
    {
        Objects.requireNonNull(state, "state must not be null");
        Objects.requireNonNull(record, "record must not be null");
        if (!state.sessionId.equals(record.sessionId()))
        {
            throw new IllegalArgumentException("record belongs to a different session");
        }
        if (state.traceId != null && !state.traceId.equals(record.traceId()))
        {
            throw new IllegalArgumentException("record belongs to a different trace");
        }
        if (state.traceId == null)
        {
            state.traceId = record.traceId();
        }
        if (state.startedAt == null)
        {
            state.startedAt = record.timestamp();
        }

        updateState(state, record);
        ExecutionActivity activity = projectActivity(state, record);
        ActiveExecutionSnapshot snapshot = snapshot(state, record);
        boolean heldTerminal = record.recordType() == TraceRecordType.TRACE_COMPLETED;
        return new Projection(snapshot, heldTerminal ? null : activity, heldTerminal ? activity : null);
    }

    private void updateState(ExecutionProjectionState state, TraceRecord record)
    {
        TraceRecordType type = record.recordType();
        if (type == TraceRecordType.FRAME_OPENED)
        {
            if (record.frameType() == TraceFrameType.ROOT_MISSION && state.frames.isEmpty()
                    && !state.entrySkill.equals(record.route()))
            {
                throw new IllegalArgumentException("top-level root route does not match entry skill");
            }
            state.openFrame(record.frameId(), record.frameType(), record.route());
            if (record.frameType() == TraceFrameType.ROOT_MISSION
                    || record.frameType() == TraceFrameType.SKILL_EXECUTION)
            {
                state.usage = state.usage.incrementSkillInvocations();
            }
        }
        else if (type == TraceRecordType.FRAME_CLOSED)
        {
            state.closeFrame(record.frameId());
        }
        else if (type == TraceRecordType.TOOL_CALL_STARTED)
        {
            state.usage = state.usage.incrementToolInvocations();
        }
        else if (type == TraceRecordType.PLAN_RETRY_REQUESTED)
        {
            state.usage = state.usage.incrementLinterRetries();
        }
        else if (type == TraceRecordType.MODEL_RESPONSE_RECEIVED)
        {
            state.usage = addModelUsage(state.usage, record.metadata().get("usage"));
        }
        else if (type == TraceRecordType.MODEL_REQUEST_SENT)
        {
            state.usage = state.usage.incrementProviderAttempts();
        }
        else if (type == TraceRecordType.TRACE_COMPLETED)
        {
            state.outcome = enumValue(record.metadata().get("outcome"), TraceOutcome.class);
            SessionUsageSnapshot terminalUsage = sessionUsage(record.metadata().get("sessionUsageSnapshot"));
            if (terminalUsage != null)
            {
                state.usage = terminalUsage;
            }
        }

        state.phase = phase(type);
        state.summary = summary(record);
    }

    private ExecutionActivity projectActivity(ExecutionProjectionState state, TraceRecord record)
    {
        if (record.recordType() == TraceRecordType.FRAME_OPENED
                || record.recordType() == TraceRecordType.FRAME_CLOSED)
        {
            if (record.frameType() != TraceFrameType.SKILL_EXECUTION)
            {
                return null;
            }
        }
        else if (!VISIBLE.contains(record.recordType()))
        {
            return null;
        }

        ExecutionActivityKind kind = activityKind(record);
        Map<String, Object> details = boundedDetails(record);
        String summary = ExecutionObservationLimits.truncate(
                summary(record), ExecutionObservationLimits.SUMMARY_CODE_POINTS);
        String executionStatus = record.recordType() == TraceRecordType.TRACE_COMPLETED
                ? state.outcome == null ? null : state.outcome.name()
                : "ACTIVE";
        int weight = retainedWeight(record, kind, executionStatus, summary, details);
        while (weight > ExecutionObservationLimits.ACTIVITY_UTF8_BYTES && !details.isEmpty())
        {
            LinkedHashMap<String, Object> reduced = new LinkedHashMap<>(details);
            String last = null;
            for (String key : reduced.keySet())
            {
                last = key;
            }
            reduced.remove(last);
            details = Map.copyOf(reduced);
            weight = retainedWeight(record, kind, executionStatus, summary, details);
        }
        if (weight > ExecutionObservationLimits.ACTIVITY_UTF8_BYTES)
        {
            summary = "";
            weight = retainedWeight(record, kind, executionStatus, summary, details);
        }
        if (weight > ExecutionObservationLimits.ACTIVITY_UTF8_BYTES)
        {
            throw new IllegalStateException("Activity structural identity exceeds retained bound");
        }
        return new ExecutionActivity(
                0L,
                record.sessionId(),
                record.traceId(),
                record.sequence(),
                record.timestamp(),
                kind,
                record.frameId(),
                record.parentFrameId(),
                record.frameType(),
                record.route(),
                executionStatus,
                summary,
                details,
                Math.max(1, weight));
    }

    private ActiveExecutionSnapshot snapshot(ExecutionProjectionState state, TraceRecord record)
    {
        List<ActiveExecutionSnapshot.FramePathEntry> fullPath = new ArrayList<>(state.frames.values());
        int total = fullPath.size();
        boolean truncated = total > ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES;
        List<ActiveExecutionSnapshot.FramePathEntry> retained;
        if (!truncated)
        {
            retained = List.copyOf(fullPath);
        }
        else
        {
            retained = new ArrayList<>(ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES);
            retained.add(fullPath.getFirst());
            retained.addAll(fullPath.subList(
                    total - (ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES - 1), total));
            retained = List.copyOf(retained);
        }
        return new ActiveExecutionSnapshot(
                record.sessionId(),
                record.traceId(),
                0L,
                record.sequence(),
                state.startedAt,
                record.timestamp(),
                state.entrySkill,
                state.phase,
                state.summary,
                retained,
                total,
                truncated,
                state.usage,
                state.outcome);
    }

    private Map<String, Object> boundedDetails(TraceRecord record)
    {
        LinkedHashMap<String, Object> candidates = new LinkedHashMap<>();
        for (String key : DETAIL_KEYS)
        {
            addScalar(candidates, key, record.metadata().get(key));
        }
        if (record.recordType() == TraceRecordType.ERROR_RECORDED && record.data() != null && record.data().isObject())
        {
            for (String key : List.of("classification", "exceptionType", "message"))
            {
                JsonNode value = record.data().get(key);
                if (value != null && value.isValueNode())
                {
                    addScalar(candidates, key, value.isTextual() ? value.textValue() : value.asText());
                }
            }
        }

        LinkedHashMap<String, Object> bounded = new LinkedHashMap<>();
        int bytes = 0;
        for (Map.Entry<String, Object> entry : candidates.entrySet())
        {
            if (bounded.size() == ExecutionObservationLimits.DETAIL_FIELDS)
            {
                break;
            }
            int added = ExecutionObservationLimits.utf8Weight(entry.getKey())
                    + ExecutionObservationLimits.utf8Weight(String.valueOf(entry.getValue()));
            if (bytes + added > ExecutionObservationLimits.DETAIL_UTF8_BYTES)
            {
                break;
            }
            bounded.put(entry.getKey(), entry.getValue());
            bytes += added;
        }
        return Map.copyOf(bounded);
    }

    private void addScalar(Map<String, Object> target, String key, Object value)
    {
        if (value instanceof String text)
        {
            target.put(key, bounded(text));
        }
        else if (value instanceof Number || value instanceof Boolean)
        {
            target.put(key, value);
        }
        else if (value instanceof Enum<?> enumeration)
        {
            target.put(key, bounded(enumeration.name()));
        }
    }

    private int retainedWeight(
            TraceRecord record,
            ExecutionActivityKind kind,
            String executionStatus,
            String summary,
            Map<String, Object> details)
    {
        int weight = 128
                + ExecutionObservationLimits.utf8Weight(record.sessionId())
                + ExecutionObservationLimits.utf8Weight(record.traceId())
                + ExecutionObservationLimits.utf8Weight(record.frameId())
                + ExecutionObservationLimits.utf8Weight(record.parentFrameId())
                + ExecutionObservationLimits.utf8Weight(record.route())
                + ExecutionObservationLimits.utf8Weight(executionStatus)
                + ExecutionObservationLimits.utf8Weight(kind.name())
                + ExecutionObservationLimits.utf8Weight(summary);
        for (Map.Entry<String, Object> entry : details.entrySet())
        {
            weight += ExecutionObservationLimits.utf8Weight(entry.getKey())
                    + ExecutionObservationLimits.utf8Weight(String.valueOf(entry.getValue())) + 8;
        }
        return weight;
    }

    private ExecutionActivityKind activityKind(TraceRecord record)
    {
        return switch (record.recordType())
        {
            case TRACE_STARTED -> ExecutionActivityKind.TRACE_STARTED;
            case FRAME_OPENED -> ExecutionActivityKind.FRAME_OPENED;
            case FRAME_CLOSED -> ExecutionActivityKind.FRAME_CLOSED;
            case MODEL_REQUEST_SENT -> ExecutionActivityKind.MODEL_REQUEST_SENT;
            case MODEL_RESPONSE_RECEIVED -> ExecutionActivityKind.MODEL_RESPONSE_RECEIVED;
            case MODEL_ATTEMPT_FAILED -> ExecutionActivityKind.MODEL_ATTEMPT_FAILED;
            case PLAN_CREATED -> ExecutionActivityKind.PLAN_CREATED;
            case PLAN_UPDATED -> ExecutionActivityKind.PLAN_UPDATED;
            case PLAN_VALIDATION_FAILED -> ExecutionActivityKind.PLAN_VALIDATION_FAILED;
            case PLAN_RETRY_REQUESTED -> ExecutionActivityKind.PLAN_RETRY_REQUESTED;
            case TOOL_CALL_STARTED -> ExecutionActivityKind.TOOL_CALL_STARTED;
            case TOOL_CALL_COMPLETED -> ExecutionActivityKind.TOOL_CALL_COMPLETED;
            case TOOL_CALL_FAILED -> ExecutionActivityKind.TOOL_CALL_FAILED;
            case STEP_STARTED -> ExecutionActivityKind.STEP_STARTED;
            case STEP_ACTION_REJECTED -> ExecutionActivityKind.STEP_ACTION_REJECTED;
            case STEP_COMPLETED -> ExecutionActivityKind.STEP_COMPLETED;
            case ERROR_RECORDED -> ExecutionActivityKind.ERROR_RECORDED;
            case TRACE_COMPLETED -> ExecutionActivityKind.TRACE_COMPLETED;
            default -> throw new IllegalArgumentException("Record type is not visible: " + record.recordType());
        };
    }

    private String phase(TraceRecordType type)
    {
        return switch (type)
        {
            case TRACE_STARTED, TRACE_CAPTURE_POLICY_RECORDED -> "STARTING";
            case FRAME_OPENED, FRAME_METADATA_RECORDED, FRAME_CLOSED -> "EXECUTING_SKILL";
            case MODEL_REQUEST_PREPARED, MODEL_REQUEST_SENT, MODEL_RESPONSE_RECEIVED, MODEL_ATTEMPT_FAILED,
                    ADVISOR_REQUEST_MUTATION_RECORDED, ADVISOR_RESPONSE_MUTATION_RECORDED,
                    MODEL_THOUGHT_CAPTURED -> "MODEL";
            case PLAN_CREATED, PLAN_UPDATED, PLAN_VALIDATION_FAILED, PLAN_RETRY_REQUESTED,
                    PLAN_QUALITY_WARNING -> "PLANNING";
            case TOOL_CALL_STARTED, TOOL_CALL_COMPLETED, TOOL_CALL_FAILED -> "TOOL";
            case STEP_STARTED, STEP_ACTION_PROPOSED, STEP_ACTION_VALIDATED,
                    STEP_ACTION_REJECTED, STEP_COMPLETED -> "STEP";
            case ERROR_RECORDED -> "ERROR";
            case TRACE_COMPLETED -> "COMPLETED";
            default -> "EXECUTING";
        };
    }

    private String summary(TraceRecord record)
    {
        return switch (record.recordType())
        {
            case TRACE_STARTED -> "Execution started";
            case FRAME_OPENED -> "Skill execution started";
            case FRAME_CLOSED -> "Skill execution completed";
            case MODEL_REQUEST_SENT -> "Model request sent";
            case MODEL_RESPONSE_RECEIVED -> "Model response received";
            case MODEL_ATTEMPT_FAILED -> failedAttemptSummary(record);
            case PLAN_CREATED -> "Plan created";
            case PLAN_UPDATED -> "Plan updated";
            case PLAN_VALIDATION_FAILED -> "Plan validation failed";
            case PLAN_RETRY_REQUESTED -> "Plan retry requested";
            case TOOL_CALL_STARTED -> "Tool call started";
            case TOOL_CALL_COMPLETED -> "Tool call completed";
            case TOOL_CALL_FAILED -> "Tool call failed";
            case STEP_STARTED -> "Step started";
            case STEP_ACTION_REJECTED -> "Step action rejected";
            case STEP_COMPLETED -> "Step completed";
            case ERROR_RECORDED -> "Execution error recorded";
            case TRACE_COMPLETED -> "Execution completed";
            default -> record.recordType().name().toLowerCase().replace('_', ' ');
        };
    }

    private String failedAttemptSummary(TraceRecord record)
    {
        Object number = record.metadata().get("providerAttemptNumber");
        if ("RETRY".equals(record.metadata().get("retryDecision")))
        {
            return "Provider attempt " + number + " failed; retrying in "
                    + record.metadata().getOrDefault("retryDelayMillis", 0) + " ms";
        }
        return "Provider attempt " + number + " failed";
    }

    private SessionUsageSnapshot addModelUsage(SessionUsageSnapshot current, Object value)
    {
        if (!(value instanceof Map<?, ?> usage))
        {
            return current.recordModelUsage(new com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord(
                    0, 0, 0, UsagePrecision.UNAVAILABLE, null));
        }
        int prompt = integer(usage.get("promptUnits"));
        int completion = integer(usage.get("completionUnits"));
        int total = Math.max(integer(usage.get("totalUnits")), Math.max(prompt, completion));
        UsagePrecision precision = enumValue(usage.get("precision"), UsagePrecision.class);
        return current.recordModelUsage(new com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord(
                prompt, completion, total, precision == null ? UsagePrecision.UNAVAILABLE : precision, null));
    }

    private SessionUsageSnapshot sessionUsage(Object value)
    {
        if (value instanceof SessionUsageSnapshot snapshot)
        {
            return snapshot;
        }
        if (!(value instanceof Map<?, ?> map))
        {
            return null;
        }
        return new SessionUsageSnapshot(
                integer(map.get("skillInvocations")),
                integer(map.get("toolInvocations")),
                integer(map.get("linterRetries")),
                integer(map.get("modelCalls")),
                integer(map.get("providerAttempts")),
                integer(map.get("promptUnits")),
                integer(map.get("completionUnits")),
                integer(map.get("totalUnits")),
                integer(map.get("exactModelResponses")),
                integer(map.get("heuristicModelResponses")),
                integer(map.get("unavailableModelResponses")));
    }

    private int integer(Object value)
    {
        return value instanceof Number number ? Math.max(0, number.intValue()) : 0;
    }

    private <E extends Enum<E>> E enumValue(Object value, Class<E> type)
    {
        if (type.isInstance(value))
        {
            return type.cast(value);
        }
        if (value instanceof String text)
        {
            try
            {
                return Enum.valueOf(type, text);
            }
            catch (IllegalArgumentException ignored)
            {
                return null;
            }
        }
        return null;
    }

    private String bounded(String value)
    {
        return ExecutionObservationLimits.truncate(value, ExecutionObservationLimits.TEXT_CODE_POINTS);
    }

    record Projection(
            ActiveExecutionSnapshot snapshot,
            ExecutionActivity activity,
            ExecutionActivity heldTerminal)
    {
    }
}
