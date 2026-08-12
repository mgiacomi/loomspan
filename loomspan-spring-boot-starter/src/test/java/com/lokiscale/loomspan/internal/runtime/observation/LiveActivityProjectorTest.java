package com.lokiscale.loomspan.internal.runtime.observation;

import com.fasterxml.jackson.databind.node.TextNode;
import com.lokiscale.loomspan.internal.core.TraceFrameType;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.util.EnumSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

class LiveActivityProjectorTest
{
    private static final Set<TraceRecordType> VISIBLE = EnumSet.of(
            TraceRecordType.TRACE_STARTED,
            TraceRecordType.FRAME_OPENED,
            TraceRecordType.FRAME_CLOSED,
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

    @Test
    void projectsExactlyTheSettledVisibleRecordKinds()
    {
        LiveActivityProjector projector = new LiveActivityProjector();

        for (TraceRecordType type : TraceRecordType.values())
        {
            ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
            TraceFrameType frameType = type == TraceRecordType.FRAME_OPENED
                    || type == TraceRecordType.FRAME_CLOSED
                    ? TraceFrameType.SKILL_EXECUTION
                    : null;
            Map<String, Object> metadata = type == TraceRecordType.TRACE_COMPLETED
                    ? Map.of(
                            "outcome", "SUCCEEDED",
                            "sessionUsageSnapshot", SessionUsageSnapshot.empty())
                    : Map.of();

            LiveActivityProjector.Projection projection = projector.project(
                    state, record(type, 1, frameType, metadata, null));

            if (type == TraceRecordType.TRACE_COMPLETED)
            {
                assertThat(projection.activity()).as(type.name()).isNull();
                assertThat(projection.heldTerminal()).as(type.name()).isNotNull();
                assertThat(projection.heldTerminal().kind().name()).isEqualTo(type.name());
            }
            else if (VISIBLE.contains(type))
            {
                assertThat(projection.activity()).as(type.name()).isNotNull();
                assertThat(projection.activity().kind().name()).isEqualTo(type.name());
            }
            else
            {
                assertThat(projection.activity()).as(type.name()).isNull();
                assertThat(projection.heldTerminal()).as(type.name()).isNull();
            }
        }
    }

    @Test
    void frameVisibilityIsLimitedToSkillExecutionButAllFramesUpdatePath()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");

        LiveActivityProjector.Projection root = projector.project(
                state, record(TraceRecordType.FRAME_OPENED, 1, TraceFrameType.ROOT_MISSION, Map.of(), null));
        LiveActivityProjector.Projection model = projector.project(
                state, record(TraceRecordType.FRAME_OPENED, 2, TraceFrameType.MODEL_CALL, Map.of(), null));

        assertThat(root.activity()).isNull();
        assertThat(model.activity()).isNull();
        assertThat(model.snapshot().activePath()).hasSize(2);
        assertThat(model.snapshot().entrySkill()).isEqualTo("route");
    }

    @Test
    void boundsPathTextAndDoesNotRetainLogicalPayload()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        String route = "\uD83D\uDE00".repeat(300);
        ExecutionProjectionState state = new ExecutionProjectionState("session", route);
        LiveActivityProjector.Projection projection = null;

        for (int index = 0; index < 70; index++)
        {
            projection = projector.project(state, new TraceRecord(
                    "trace", "session", index + 1L, Instant.parse("2026-07-24T12:00:00Z"),
                    TraceRecordType.FRAME_OPENED, "frame-" + index, index == 0 ? null : "frame-" + (index - 1),
                    index == 0 ? TraceFrameType.ROOT_MISSION : TraceFrameType.SKILL_EXECUTION,
                    route, "thread", Map.of(), TextNode.valueOf("SECRET-PAYLOAD")));
        }

        assertThat(projection).isNotNull();
        assertThat(projection.snapshot().activePath())
                .hasSize(ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES);
        assertThat(projection.snapshot().activePathTruncated()).isTrue();
        assertThat(projection.snapshot().totalFrameDepth()).isEqualTo(70);
        assertThat(projection.snapshot().activePath().getFirst().frameId()).isEqualTo("frame-0");
        assertThat(projection.snapshot().activePath().getLast().frameId()).isEqualTo("frame-69");
        assertThat(projection.snapshot().activePath().getLast().route().codePointCount(
                0, projection.snapshot().activePath().getLast().route().length()))
                .isEqualTo(ExecutionObservationLimits.TEXT_CODE_POINTS);
        assertThat(projection.toString()).doesNotContain("SECRET-PAYLOAD");
    }

    @Test
    void terminalUsageReplacesDerivedCountsAndCompletionIsHeld()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        projector.project(state, record(
                TraceRecordType.TOOL_CALL_STARTED, 1, null, Map.of("capabilityName", "tool"), null));
        SessionUsageSnapshot terminal = new SessionUsageSnapshot(4, 5, 6, 7, 0, 8, 9, 17, 1, 2, 4);

        LiveActivityProjector.Projection projection = projector.project(state, record(
                TraceRecordType.TRACE_COMPLETED,
                2,
                null,
                Map.of("outcome", "FAILED", "sessionUsageSnapshot", terminal),
                null));

        assertThat(projection.snapshot().usage()).isEqualTo(terminal);
        assertThat(projection.snapshot().outcome()).isEqualTo(com.lokiscale.loomspan.internal.core.TraceOutcome.FAILED);
        assertThat(projection.activity()).isNull();
        assertThat(projection.heldTerminal().kind()).isEqualTo(ExecutionActivityKind.TRACE_COMPLETED);
        assertThat(projection.heldTerminal().executionStatus()).isEqualTo("FAILED");
        assertThat(projection.heldTerminal().retainedWeight())
                .isEqualTo(expectedRetainedWeight(projection.heldTerminal()));
    }

    private static int expectedRetainedWeight(ExecutionActivity activity)
    {
        int weight = 128
                + ExecutionObservationLimits.utf8Weight(activity.sessionId())
                + ExecutionObservationLimits.utf8Weight(activity.traceId())
                + ExecutionObservationLimits.utf8Weight(activity.frameId())
                + ExecutionObservationLimits.utf8Weight(activity.parentFrameId())
                + ExecutionObservationLimits.utf8Weight(activity.route())
                + ExecutionObservationLimits.utf8Weight(activity.executionStatus())
                + ExecutionObservationLimits.utf8Weight(activity.kind().name())
                + ExecutionObservationLimits.utf8Weight(activity.summary());
        for (Map.Entry<String, Object> entry : activity.details().entrySet())
        {
            weight += ExecutionObservationLimits.utf8Weight(entry.getKey())
                    + ExecutionObservationLimits.utf8Weight(String.valueOf(entry.getValue())) + 8;
        }
        return Math.max(1, weight);
    }

    @Test
    void projectsParentIdentityAndTruthfulExecutionStatus()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        TraceRecord nested = new TraceRecord(
                "trace", "session", 1, Instant.parse("2026-07-24T12:00:00Z"),
                TraceRecordType.TOOL_CALL_STARTED, "child-frame", "parent-frame",
                TraceFrameType.TOOL_INVOCATION, "route", "thread", Map.of(), null);

        ExecutionActivity activity = projector.project(state, nested).activity();

        assertThat(activity.parentFrameId()).isEqualTo("parent-frame");
        assertThat(activity.executionStatus()).isEqualTo("ACTIVE");
    }

    @Test
    void derivesCountsAndNormalizedModelUsageFromCanonicalFacts()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        projector.project(state, record(
                TraceRecordType.FRAME_OPENED, 1, TraceFrameType.ROOT_MISSION, Map.of(), null));
        projector.project(state, record(
                TraceRecordType.TOOL_CALL_STARTED, 2, null, Map.of(), null));
        projector.project(state, record(
                TraceRecordType.PLAN_RETRY_REQUESTED, 3, null, Map.of(), null));
        LiveActivityProjector.Projection projection = projector.project(state, record(
                TraceRecordType.MODEL_RESPONSE_RECEIVED,
                4,
                null,
                Map.of("usage", Map.of(
                        "promptUnits", 2,
                        "completionUnits", 3,
                        "totalUnits", 5,
                        "precision", "EXACT")),
                null));

        assertThat(projection.snapshot().usage())
                .isEqualTo(new SessionUsageSnapshot(1, 1, 1, 1, 0, 2, 3, 5, 1, 0, 0));
    }

    @Test
    void toolStartActivityExcludesArgumentsAndCountsOnce()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        LiveActivityProjector.Projection projection = projector.project(state, record(
                TraceRecordType.TOOL_CALL_STARTED,
                1,
                TraceFrameType.TOOL_INVOCATION,
                Map.of("capabilityName", "lookupCustomer", "linkedTaskId", "task-1"),
                TextNode.valueOf("{\"details\":{\"arguments\":{\"password\":\"must-not-render\"}}}")));

        assertThat(projection.snapshot().usage().toolInvocations()).isEqualTo(1);
        assertThat(projection.activity().summary()).isEqualTo("Tool call started");
        assertThat(projection.activity().details())
                .containsEntry("capabilityName", "lookupCustomer")
                .containsEntry("linkedTaskId", "task-1")
                .doesNotContainKey("arguments");
        assertThat(projection.activity().toString()).doesNotContain("must-not-render", "password");
    }

    @Test
    void activityDtoEnforcesTextDetailAndEnvelopeBounds()
    {
        java.util.LinkedHashMap<String, Object> thirtyTwo = new java.util.LinkedHashMap<>();
        for (int index = 0; index < 32; index++)
        {
            thirtyTwo.put("k" + index, "v");
        }
        ExecutionActivity accepted = new ExecutionActivity(
                0, "session", "trace", 1L, Instant.parse("2026-07-24T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, null,
                "😀".repeat(600), thirtyTwo, ExecutionObservationLimits.ACTIVITY_UTF8_BYTES);
        assertThat(accepted.summary().codePointCount(0, accepted.summary().length()))
                .isEqualTo(ExecutionObservationLimits.SUMMARY_CODE_POINTS);
        assertThat(accepted.details()).hasSize(ExecutionObservationLimits.DETAIL_FIELDS);

        thirtyTwo.put("overflow", "v");
        org.assertj.core.api.Assertions.assertThatThrownBy(() -> new ExecutionActivity(
                0, "session", "trace", 1L, Instant.parse("2026-07-24T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, null,
                "summary", thirtyTwo, 100))
                .isInstanceOf(IllegalArgumentException.class);
        org.assertj.core.api.Assertions.assertThatThrownBy(() -> new ExecutionActivity(
                0, "session", "trace", 1L, Instant.parse("2026-07-24T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, null,
                "summary", Map.of(), ExecutionObservationLimits.ACTIVITY_UTF8_BYTES + 1))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void providerRetryActivityContainsOnlyBoundedNeutralFacts()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        LiveActivityProjector.Projection projection = projector.project(state, record(
                TraceRecordType.MODEL_ATTEMPT_FAILED, 1, null,
                Map.of("providerAttemptNumber", 2, "attemptReason", "PROVIDER_RETRY",
                        "failureClassification", "TRANSIENT", "failureCategory", "RATE_LIMITED",
                        "retryDecision", "RETRY", "retryDelayMillis", 750,
                        "retryDelaySource", "RETRY_AFTER", "summary", "secret provider body"),
                TextNode.valueOf("secret provider body and partial assistant content")));

        assertThat(projection.activity().summary()).isEqualTo("Provider attempt 2 failed; retrying in 750 ms");
        assertThat(projection.activity().details())
                .containsEntry("failureCategory", "RATE_LIMITED")
                .containsEntry("retryDecision", "RETRY")
                .doesNotContainKey("summary");
        assertThat(projection.activity().toString())
                .doesNotContain("secret provider body", "partial assistant content");
    }

    @Test
    void enforcesExactPathAndDetailByteBoundaries()
    {
        LiveActivityProjector projector = new LiveActivityProjector();
        ExecutionProjectionState state = new ExecutionProjectionState("session", "route");
        LiveActivityProjector.Projection atPathLimit = null;
        for (int index = 0; index < ExecutionObservationLimits.ACTIVE_FRAME_PATH_ENTRIES; index++)
        {
            atPathLimit = projector.project(state, new TraceRecord(
                    "trace", "session", index + 1L, Instant.parse("2026-07-24T12:00:00Z"),
                    TraceRecordType.FRAME_OPENED, "frame-" + index, null, TraceFrameType.SKILL_EXECUTION,
                    "route", "thread", Map.of(), null));
        }
        assertThat(atPathLimit.snapshot().activePath()).hasSize(64);
        assertThat(atPathLimit.snapshot().activePathTruncated()).isFalse();
        LiveActivityProjector.Projection overPathLimit = projector.project(state, new TraceRecord(
                "trace", "session", 65L, Instant.parse("2026-07-24T12:00:00Z"),
                TraceRecordType.FRAME_OPENED, "frame-64", null, TraceFrameType.SKILL_EXECUTION,
                "route", "thread", Map.of(), null));
        assertThat(overPathLimit.snapshot().activePath()).hasSize(64);
        assertThat(overPathLimit.snapshot().activePathTruncated()).isTrue();

        java.util.LinkedHashMap<String, Object> exactBytes = new java.util.LinkedHashMap<>();
        for (int index = 0; index < 32; index++)
        {
            String prefix = "k" + index;
            exactBytes.put(prefix + "x".repeat(128 - prefix.length()), "v".repeat(128));
        }
        ExecutionActivity exact = new ExecutionActivity(
                0, "session", "trace", 1L, Instant.parse("2026-07-24T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, null,
                "summary", exactBytes, 100);
        assertThat(exact.details()).hasSize(32);

        java.util.LinkedHashMap<String, Object> overBytes = new java.util.LinkedHashMap<>(exactBytes);
        String firstKey = overBytes.keySet().iterator().next();
        overBytes.put(firstKey, "v".repeat(129));
        org.assertj.core.api.Assertions.assertThatThrownBy(() -> new ExecutionActivity(
                0, "session", "trace", 1L, Instant.parse("2026-07-24T12:00:00Z"),
                ExecutionActivityKind.TRACE_STARTED, null, null, null, null, null,
                "summary", overBytes, 100))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("byte limit");
    }

    private TraceRecord record(
            TraceRecordType type,
            long sequence,
            TraceFrameType frameType,
            Map<String, Object> metadata,
            TextNode data)
    {
        return new TraceRecord(
                "trace", "session", sequence, Instant.parse("2026-07-24T12:00:00Z"), type,
                frameType == null ? null : "frame-" + sequence, null, frameType,
                frameType == null ? null : "route", "thread", metadata, data);
    }
}
