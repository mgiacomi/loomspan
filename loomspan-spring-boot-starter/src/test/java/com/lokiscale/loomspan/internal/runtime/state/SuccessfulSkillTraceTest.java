package com.lokiscale.loomspan.internal.runtime.state;

import com.lokiscale.loomspan.internal.core.LoomspanSession;
import com.lokiscale.loomspan.internal.core.ExecutionFrame;
import com.lokiscale.loomspan.internal.core.TraceRecord;
import com.lokiscale.loomspan.internal.core.TraceRecordType;
import org.junit.jupiter.api.Test;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

class SuccessfulSkillTraceTest
{
    @Test
    void repeatedSuccessIsSetIdempotentButRetainsDistinctEvidenceEvents()
    {
        DefaultExecutionStateService state = new DefaultExecutionStateService(Clock.fixed(
                Instant.parse("2026-03-15T12:00:00Z"), ZoneOffset.UTC));
        LoomspanSession session = com.lokiscale.loomspan.internal.core.TestLoomspanSessions.withId("trace", "test.entry", 3);
        ExecutionFrame frame = state.openMissionFrame(session, "handleIncident", Map.of());

        state.recordSuccessfulSkill(session, "investigateNetwork", "task-1", false);
        state.recordSuccessfulSkill(session, "investigateNetwork", "task-2", false);
        state.closeMissionFrame(session, frame);

        assertThat(session.getSuccessfulDirectSkills()).containsExactly("investigateNetwork");
        List<TraceRecord> records = new ArrayList<>();
        session.readTraceRecords(records::add);
        assertThat(records).filteredOn(record -> record.recordType() == TraceRecordType.EVIDENCE_RECORDED)
                .hasSize(2)
                .allSatisfy(record ->
                {
                    assertThat(record.data().path("successfulSkill").asText()).isEqualTo("investigateNetwork");
                    assertThat(record.data().path("successfulDirectSkills")).hasSize(1);
                    assertThat(record.data().has("evidenceTypes")).isFalse();
                    assertThat(record.data().has("ledger")).isFalse();
                });
    }
}
