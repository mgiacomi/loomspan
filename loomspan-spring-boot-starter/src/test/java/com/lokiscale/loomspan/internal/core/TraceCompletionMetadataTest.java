package com.lokiscale.loomspan.internal.core;

import com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class TraceCompletionMetadataTest {

    @Test
    void metadataSerializesSessionUsageSnapshotWithTraceContractFieldNames() {
        SessionUsageSnapshot snapshot = new SessionUsageSnapshot(
                2, 1, 0, 3, 0, 150, 40, 190, 3, 0, 0);
        TraceCompletion completion = new TraceCompletion(
                TraceOutcome.SUCCEEDED, snapshot, null, Map.of());

        Map<String, Object> metadata = completion.metadata();

        @SuppressWarnings("unchecked")
        Map<String, Object> serialized = (Map<String, Object>) metadata.get("sessionUsageSnapshot");
        assertThat(serialized).isNotNull();
        assertThat(serialized).containsKey("totalUnits");
        assertThat(serialized.get("totalUnits")).isEqualTo(190);
        assertThat(serialized).doesNotContainKey("usageUnits");
        assertThat(serialized.get("promptUnits")).isEqualTo(150);
        assertThat(serialized.get("completionUnits")).isEqualTo(40);
        assertThat(serialized.get("modelCalls")).isEqualTo(3);
        assertThat(serialized.get("skillInvocations")).isEqualTo(2);
        assertThat(serialized.get("toolInvocations")).isEqualTo(1);
        assertThat(serialized.get("exactModelResponses")).isEqualTo(3);
        assertThat(serialized.get("heuristicModelResponses")).isEqualTo(0);
        assertThat(serialized.get("unavailableModelResponses")).isEqualTo(0);
    }

    @Test
    void metadataPreservesExplicitSessionUsageSnapshotFromDetails() {
        Map<String, Object> explicitSnapshot = Map.of(
                "promptUnits", 100,
                "completionUnits", 20,
                "totalUnits", 120);
        TraceCompletion completion = new TraceCompletion(
                TraceOutcome.SUCCEEDED,
                SessionUsageSnapshot.empty(),
                null,
                Map.of("sessionUsageSnapshot", explicitSnapshot));

        Map<String, Object> metadata = completion.metadata();

        assertThat(metadata.get("sessionUsageSnapshot")).isSameAs(explicitSnapshot);
        assertThat(metadata.get("outcome")).isEqualTo("SUCCEEDED");
    }

    @Test
    void metadataIncludesTerminalFailureIdForFailedTrace() {
        TraceCompletion completion = new TraceCompletion(
                TraceOutcome.FAILED,
                SessionUsageSnapshot.empty(),
                "failure-123",
                Map.of());

        Map<String, Object> metadata = completion.metadata();

        assertThat(metadata.get("terminalFailureId")).isEqualTo("failure-123");
        assertThat(metadata.get("outcome")).isEqualTo("FAILED");
    }

    @Test
    void metadataIncludesDetailsAlongsideSessionUsageSnapshot() {
        SessionUsageSnapshot snapshot = new SessionUsageSnapshot(
                1, 0, 0, 1, 0, 10, 4, 14, 1, 0, 0);
        TraceCompletion completion = new TraceCompletion(
                TraceOutcome.SUCCEEDED,
                snapshot,
                null,
                Map.of("skillName", "expenseLookup", "remainingFrames", 0));

        Map<String, Object> metadata = completion.metadata();

        assertThat(metadata.get("skillName")).isEqualTo("expenseLookup");
        assertThat(metadata.get("remainingFrames")).isEqualTo(0);
        assertThat(metadata.get("outcome")).isEqualTo("SUCCEEDED");
        assertThat(metadata).containsKey("sessionUsageSnapshot");
    }
}
