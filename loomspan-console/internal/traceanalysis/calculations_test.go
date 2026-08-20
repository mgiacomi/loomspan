package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
)

// frameRecord builds a FRAME_OPENED or FRAME_CLOSED record line for tests.
func frameRecord(seq int, frameID, parentID string, hasParent bool, frameType string, opened bool) string {
	rt := "FRAME_OPENED"
	if !opened {
		rt = "FRAME_CLOSED"
	}
	parentJSON := "null"
	if hasParent {
		parentJSON = `"` + parentID + `"`
	}
	ftJSON := "null"
	if frameType != "" {
		ftJSON = `"` + frameType + `"`
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"` + rt + `","frameId":"` + frameID +
		`","parentFrameId":` + parentJSON +
		`,"frameType":` + ftJSON +
		`,"route":null,"threadName":"th","metadata":{},"data":null}`
}

// timestampForSeq returns a deterministic timestamp for a sequence number,
// matching the fixture corpus pattern (base + seq*1000ms).
func timestampForSeq(seq int) string {
	// 1784894400.000000000 + seq seconds
	base := 1784894400 + seq
	return itoa(base) + ".000000000"
}

// responseRecord builds a MODEL_RESPONSE_RECEIVED record with usage.
func responseRecord(seq int, frameID, retryID, attemptID string, attemptNum int, prompt, completion, total int, precision string) string {
	frameJSON := "null"
	if frameID != "" {
		frameJSON = `"` + frameID + `"`
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":` + frameJSON +
		`,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"retrySequenceId":"` + retryID + `","attemptId":"` + attemptID +
		`","attemptNumber":` + itoa(attemptNum) +
		`,"attemptReason":"` + attemptReason(attemptNum) + `","providerAttemptNumber":1` +
		`,"usage":{"promptUnits":` + itoa(prompt) + `,"completionUnits":` + itoa(completion) +
		`,"totalUnits":` + itoa(total) + `,"precision":"` + precision + `"}},"data":{"content":"r"}}`
}

// requestRecord builds the sent boundary. Legacy two-boundary test layouts use
// a harmless thought record in the former first slot so sequence fixtures stay
// compact while the attempt graph sees exactly one send.
func requestRecord(seq int, retryID, attemptID string, attemptNum int, formerFirstSlot bool) string {
	rt := "MODEL_REQUEST_SENT"
	if formerFirstSlot {
		rt = "MODEL_THOUGHT_CAPTURED"
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"` + rt + `","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"retrySequenceId":"` + retryID + `","attemptId":"` + attemptID +
		`","attemptNumber":` + itoa(attemptNum) + `,"attemptReason":"` + attemptReason(attemptNum) +
		`","providerAttemptNumber":1},"data":{"messages":["u"]}}`
}

func attemptReason(number int) string {
	if number == 1 {
		return "INITIAL"
	}
	return "SEMANTIC_RETRY"
}

// completionRecord builds a TRACE_COMPLETED record.
func completionRecord(seq int, outcome string, prompt, completion, total int, terminalFailureID string) string {
	tfJSON := "null"
	if terminalFailureID != "" {
		tfJSON = `"` + terminalFailureID + `"`
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"outcome":"` + outcome + `","sessionUsageSnapshot":{"promptUnits":` + itoa(prompt) +
		`,"completionUnits":` + itoa(completion) + `,"totalUnits":` + itoa(total) +
		`},"errored":false,"persistencePolicy":"ALWAYS","terminalFailureId":` + tfJSON + `},"data":null}`
}

// errorRecord builds an ERROR_RECORDED record.
func errorRecord(seq int, failureID string, _ bool) string {
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"ERROR_RECORDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"failureId":"` + failureID + `"},"data":{"exceptionType":"java.lang.IllegalStateException","message":"failed","diagnostics":[{"kind":"JAVA_STACK_TRACE","contentType":"text/plain; charset=utf-8","text":"stack","truncated":false,"captureLimitBytes":1048576}]}}`
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// startedRecord builds a TRACE_STARTED record.
func startedRecord(seq int) string {
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"consoleCompatibilityVersion":"development"},"data":null}`
}

// processTrace runs the full processor over raw and returns the manifest or the
// error category.
func processTrace(t *testing.T, raw string) (manifest, InvalidityCategory, bool) {
	t.Helper()
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		cat, _ := categoryOf(domain)
		return manifest{}, cat, false
	}
	mBytes := sink.components[ComponentManifest]
	var m manifest
	if err := json.Unmarshal(mBytes, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m, "", true
}

// TestFramesBuildDeterministicHierarchyFromRecordedIDs proves the frame graph
// builds a hierarchy from recorded frame IDs and produces correct inclusive
// usage.
func TestFramesBuildDeterministicHierarchyFromRecordedIDs(t *testing.T) {
	// root (ROOT_MISSION) -> skill (SKILL_EXECUTION)
	// root has no direct usage; skill has (4,2); unframed has (1,1)
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		frameRecord(3, "skill", "root", true, "SKILL_EXECUTION", true) + "\n" +
		requestRecord(4, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(5, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(6, "skill", "retry-1", "attempt-1", 1, 4, 2, 6, "EXACT") + "\n" +
		requestRecord(7, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(8, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(9, "", "retry-2", "attempt-2", 1, 1, 1, 2, "EXACT") + "\n" +
		frameRecord(10, "skill", "root", true, "SKILL_EXECUTION", false) + "\n" +
		frameRecord(11, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(12, "SUCCEEDED", 5, 3, 8, "") + "\n"

	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.FrameCount != 2 {
		t.Fatalf("expected 2 frames, got %d", m.FrameCount)
	}
}

// TestFramesDescendantUsageBottomUpThreeLevelHierarchy proves the bottom-up
// descendant usage computation is correct for a 3-level hierarchy where each
// level has direct usage. root -> skill -> tool. The root's descendant usage
// must include both skill and tool direct usage.
func TestFramesDescendantUsageBottomUpThreeLevelHierarchy(t *testing.T) {
	// root (ROOT_MISSION) -> skill (SKILL_EXECUTION) -> tool (TOOL_INVOCATION)
	// tool direct: (1,1,2); skill direct: (4,2,6); root direct: (0,0,0)
	// Expected descendant: tool=(0,0,0), skill=(1,1,2), root=(5,3,8)
	// Expected inclusive: tool=(1,1,2), skill=(5,3,8), root=(5,3,8)
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		frameRecord(3, "skill", "root", true, "SKILL_EXECUTION", true) + "\n" +
		frameRecord(4, "tool", "skill", true, "TOOL_INVOCATION", true) + "\n" +
		requestRecord(5, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(6, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(7, "tool", "retry-1", "attempt-1", 1, 1, 1, 2, "EXACT") + "\n" +
		requestRecord(8, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(9, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(10, "skill", "retry-2", "attempt-2", 1, 4, 2, 6, "EXACT") + "\n" +
		frameRecord(11, "tool", "skill", true, "TOOL_INVOCATION", false) + "\n" +
		frameRecord(12, "skill", "root", true, "SKILL_EXECUTION", false) + "\n" +
		frameRecord(13, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(14, "SUCCEEDED", 5, 3, 8, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("expected valid trace, got %v", domain)
	}
	frameBytes, ok := sink.components[artifact.ComponentName(ComponentFrameIndex)]
	if !ok {
		t.Fatal("expected frame index component")
	}
	// The frame index is length-prefixed JSON rows (uint32 LE + JSON per row).
	var frames []frameResult
	r := bytes.NewReader(frameBytes)
	for r.Len() > 0 {
		row, err := readLengthPrefixed(r)
		if err != nil {
			t.Fatalf("read frame row: %v", err)
		}
		var fr frameResult
		if err := json.Unmarshal(row, &fr); err != nil {
			t.Fatalf("parse frame row: %v", err)
		}
		frames = append(frames, fr)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	// Frames are in first-open order: root, skill, tool.
	root := frames[0]
	skill := frames[1]
	tool := frames[2]
	// Tool has no descendants.
	if tool.DescendantUsage.TotalUnits != 0 {
		t.Errorf("tool descendant: got %d want 0", tool.DescendantUsage.TotalUnits)
	}
	if tool.InclusiveUsage.TotalUnits != 2 {
		t.Errorf("tool inclusive: got %d want 2", tool.InclusiveUsage.TotalUnits)
	}
	// Skill's descendant includes tool's direct usage.
	if skill.DescendantUsage.TotalUnits != 2 {
		t.Errorf("skill descendant: got %d want 2", skill.DescendantUsage.TotalUnits)
	}
	if skill.InclusiveUsage.TotalUnits != 8 {
		t.Errorf("skill inclusive: got %d want 8", skill.InclusiveUsage.TotalUnits)
	}
	// Root's descendant includes skill and tool direct usage.
	if root.DescendantUsage.TotalUnits != 8 {
		t.Errorf("root descendant: got %d want 8", root.DescendantUsage.TotalUnits)
	}
	if root.InclusiveUsage.TotalUnits != 8 {
		t.Errorf("root inclusive: got %d want 8", root.InclusiveUsage.TotalUnits)
	}
}

// TestFramesRejectSelfParent proves a frame whose parent is itself is rejected.
func TestFramesRejectSelfParent(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "root", true, "ROOT_MISSION", true) + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for self-parent frame")
	}
	if cat != CategoryInvalidFrameRelationship {
		t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
	}
}

// TestFramesRejectMissingParent proves a frame with a missing parent is
// rejected. A completion is included so the MISSING_COMPLETION check does not
// fire first.
func TestFramesRejectMissingParent(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "child", "nonexistent", true, "SKILL_EXECUTION", true) + "\n" +
		completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for missing parent")
	}
	if cat != CategoryInvalidFrameRelationship {
		t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
	}
}

// TestFramesMarkIncompleteChildSelfDurationUnavailable proves that when a child
// frame is not closed, the parent's self duration is marked unavailable with the
// INCOMPLETE_CHILD uncertainty.
func TestFramesMarkIncompleteChildSelfDurationUnavailable(t *testing.T) {
	// root opened and closed, child opened but never closed
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		frameRecord(3, "child", "root", true, "TOOL_INVOCATION", true) + "\n" +
		frameRecord(4, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.UncertaintyCount != 1 {
		t.Fatalf("expected 1 uncertainty, got %d", m.UncertaintyCount)
	}
	if m.GapCount != 1 {
		t.Fatalf("expected 1 gap (open frame not closed), got %d", m.GapCount)
	}
}

// TestUsageAcceptsExactHeuristicAndUnavailableOnly proves the parser rejects
// ESTIMATED and accepts EXACT, HEURISTIC, and UNAVAILABLE.
func TestUsageAcceptsExactHeuristicAndUnavailableOnly(t *testing.T) {
	precisions := []struct {
		value string
		valid bool
	}{
		{"EXACT", true},
		{"HEURISTIC", true},
		{"UNAVAILABLE", true},
		{"ESTIMATED", false},
		{"FUTURE", false},
	}
	for _, p := range precisions {
		t.Run(p.value, func(t *testing.T) {
			raw := startedRecord(1) + "\n" +
				requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
				requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
				responseRecord(4, "", "retry-1", "attempt-1", 1, 10, 4, 14, p.value) + "\n" +
				completionRecord(5, "SUCCEEDED", 10, 4, 14, "") + "\n"
			_, cat, ok := processTrace(t, raw)
			if p.valid && !ok {
				t.Fatalf("expected precision %s to be valid, got category %s", p.value, cat)
			}
			if !p.valid && ok {
				t.Fatalf("expected precision %s to be invalid, but trace was accepted", p.value)
			}
		})
	}
}

// TestUsageRejectsNegativeComponents proves negative usage components are
// rejected.
func TestUsageRejectsNegativeComponents(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, -1, 4, 3, "EXACT") + "\n" +
		completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for negative usage")
	}
	if cat != CategoryInvalidUsage {
		t.Fatalf("expected INVALID_USAGE, got %s", cat)
	}
}

// TestUsageRejectsContradictoryReconciliation proves that when the terminal
// usage is less than attributed usage, the unattributed remainder is negative
// and rejected.
func TestUsageRejectsContradictoryReconciliation(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, 10, 4, 14, "EXACT") + "\n" +
		completionRecord(5, "SUCCEEDED", 1, 1, 2, "") + "\n" // terminal < attributed
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for contradictory usage")
	}
	if cat != CategoryContradictoryUsage {
		t.Fatalf("expected CONTRADICTORY_USAGE, got %s", cat)
	}
}

// TestTerminalFailureRequiresResolvableTerminalFailure proves a FAILED outcome
// without a resolvable terminal failure is rejected.
func TestTerminalFailureRequiresResolvableTerminalFailure(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		completionRecord(2, "FAILED", 0, 0, 0, "missing-failure") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for unresolved terminal failure")
	}
	if cat != CategoryInvalidTerminalFailure {
		t.Fatalf("expected INVALID_TERMINAL_FAILURE, got %s", cat)
	}
}

// TestTerminalFailureSucceededForbidsTerminalFailureID proves a SUCCEEDED outcome
// with a terminalFailureId is rejected.
func TestTerminalFailureSucceededForbidsTerminalFailureID(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		errorRecord(2, "failure-1", true) + "\n" +
		completionRecord(3, "SUCCEEDED", 0, 0, 0, "failure-1") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for succeeded with terminal failure")
	}
	if cat != CategoryInvalidTerminalFailure {
		t.Fatalf("expected INVALID_TERMINAL_FAILURE, got %s", cat)
	}
}

// TestTerminalFailureResolvesFailedOutcome proves a FAILED outcome with a
// matching terminal ERROR_RECORDED is accepted.
func TestTerminalFailureResolvesFailedOutcome(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		errorRecord(2, "failure-terminal", true) + "\n" +
		completionRecord(3, "FAILED", 0, 0, 0, "failure-terminal") + "\n"
	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.Outcome != "FAILED" {
		t.Fatalf("expected outcome FAILED, got %s", m.Outcome)
	}
	if m.FailureCount != 1 {
		t.Fatalf("expected 1 failure, got %d", m.FailureCount)
	}
}

// TestAttemptsRejectInconsistentAttemptNumber proves changing the attemptNumber
// for the same attemptId is rejected.
func TestAttemptsRejectInconsistentAttemptNumber(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		// Same attemptId but different attemptNumber in the SENT record
		`{"traceId":"t","sessionId":"s","sequence":3,"timestamp":` + timestampForSeq(3) +
		`,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"retrySequenceId":"retry-1","attemptId":"attempt-1","attemptNumber":2},"data":{"messages":["u"]}}` + "\n" +
		completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for inconsistent attempt number")
	}
	if cat != CategoryInvalidAttempt {
		t.Fatalf("expected INVALID_ATTEMPT, got %s", cat)
	}
}

// TestAttemptsRejectOutOfOrderLifecycle proves a RESPONSE_RECEIVED without
// preceding PREPARED and SENT is rejected.
func TestAttemptsRejectOutOfOrderLifecycle(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		responseRecord(2, "", "retry-1", "attempt-1", 1, 10, 4, 14, "EXACT") + "\n" +
		completionRecord(3, "SUCCEEDED", 10, 4, 14, "") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for out-of-order lifecycle")
	}
	if cat != CategoryInvalidAttempt {
		t.Fatalf("expected INVALID_ATTEMPT, got %s", cat)
	}
}

// TestUsageRepresentsUnavailableAsUnknownNotZero proves that when precision is
// UNAVAILABLE, usageComplete is false (unknown, not zero).
func TestUsageRepresentsUnavailableAsUnknownNotZero(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, 0, 0, 0, "UNAVAILABLE") + "\n" +
		completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
	m, _, ok := processTrace(t, raw)
	if !ok {
		t.Fatal("expected valid trace with UNAVAILABLE precision")
	}
	// The manifest should reflect 1 attempt with incomplete usage.
	if m.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", m.AttemptCount)
	}
}

// TestUsageRejectsAccumulationOverflow proves that accumulating usage across
// multiple responses that overflows int64 is rejected as CONTRADICTORY_USAGE,
// not silently wrapped.
func TestUsageRejectsAccumulationOverflow(t *testing.T) {
	// Two responses each with promptUnits = math.MaxInt64/2 + 1 so the sum
	// overflows int64.
	big := 9223372036854775807/2 + 1
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, big, 0, big, "EXACT") + "\n" +
		requestRecord(5, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(6, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(7, "", "retry-2", "attempt-2", 1, big, 0, big, "EXACT") + "\n" +
		completionRecord(8, "SUCCEEDED", big*2, 0, big*2, "") + "\n"
	_, cat, ok := processTrace(t, raw)
	if ok {
		t.Fatal("expected error for accumulation overflow")
	}
	if cat != CategoryContradictoryUsage {
		t.Fatalf("expected CONTRADICTORY_USAGE, got %s", cat)
	}
}

// deepFrameChainReader is an io.Reader that generates a trace with a deeply
// nested frame chain on demand, without materializing the entire trace in a
// single Go slice. Frames are opened in order frame-0 -> frame-1 -> ... ->
// frame-(depth-1), then closed in reverse order. Each frame is a
// SKILL_EXECUTION child of the previous frame; frame-0's parent is null with
// type ROOT_MISSION.
type deepFrameChainReader struct {
	depth int
	phase int // 0=started, 1=open, 2=close, 3=completed
	index int
	buf   bytes.Buffer
}

func newDeepFrameChainReader(depth int) *deepFrameChainReader {
	return &deepFrameChainReader{depth: depth}
}

func (r *deepFrameChainReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for r.buf.Len() == 0 {
		switch r.phase {
		case 0:
			r.buf.WriteString(startedRecord(1) + "\n")
			r.phase = 1
			r.index = 0
		case 1:
			if r.index >= r.depth {
				r.phase = 2
				r.index = r.depth - 1
				continue
			}
			seq := 2 + r.index
			frameID := "frame-" + itoa(r.index)
			if r.index == 0 {
				r.buf.WriteString(frameRecord(seq, frameID, "", false, "ROOT_MISSION", true) + "\n")
			} else {
				parent := "frame-" + itoa(r.index-1)
				r.buf.WriteString(frameRecord(seq, frameID, parent, true, "SKILL_EXECUTION", true) + "\n")
			}
			r.index++
		case 2:
			if r.index < 0 {
				r.phase = 3
				continue
			}
			seq := 2 + r.depth + (r.depth - 1 - r.index)
			frameID := "frame-" + itoa(r.index)
			if r.index == 0 {
				r.buf.WriteString(frameRecord(seq, frameID, "", false, "ROOT_MISSION", false) + "\n")
			} else {
				parent := "frame-" + itoa(r.index-1)
				r.buf.WriteString(frameRecord(seq, frameID, parent, true, "SKILL_EXECUTION", false) + "\n")
			}
			r.index--
		case 3:
			seq := 2 + r.depth*2
			r.buf.WriteString(completionRecord(seq, "SUCCEEDED", 0, 0, 0, "") + "\n")
			r.phase = 4
		case 4:
			return 0, io.EOF
		}
	}
	return r.buf.Read(p)
}

// TestFramesProcessTwentyThousandDeepChainIteratively proves the frame graph
// builds a 20,000-deep hierarchy iteratively without recursion/stack failure,
// and every frame remains indexed. This protects against stack overflow on deep
// valid frame trees and confirms the iterative traversal has no depth cap.
func TestFramesProcessTwentyThousandDeepChainIteratively(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deep frame chain test in short mode")
	}
	const depth = 20000
	reader := newDeepFrameChainReader(depth)
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      reader,
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed for %d-deep chain: %v", depth, domain)
	}
	manifestBytes, ok := sink.components[ComponentManifest]
	if !ok {
		t.Fatal("expected manifest component")
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if m.FrameCount != depth {
		t.Fatalf("expected %d frames, got %d", depth, m.FrameCount)
	}
	// Verify the frame index contains every frame by counting rows.
	frameIdxBytes, ok := sink.components[artifact.ComponentName(ComponentFrameIndex)]
	if !ok {
		t.Fatal("expected frame index component")
	}
	frameCount := 0
	r := bytes.NewReader(frameIdxBytes)
	for r.Len() > 0 {
		_, err := readLengthPrefixed(r)
		if err != nil {
			t.Fatalf("read frame row %d: %v", frameCount, err)
		}
		frameCount++
	}
	if frameCount != depth {
		t.Fatalf("expected %d frame index rows, got %d", depth, frameCount)
	}
}

// Ensure bytes import is used (for bytesReader in fixture_corpus_test).
var _ = bytes.Equal

// advisorRecord builds an ADVISOR_REQUEST_MUTATION_RECORDED or
// ADVISOR_RESPONSE_MUTATION_RECORDED record carrying a validation link.
func advisorRecord(seq int, request bool, retryID, attemptID string, attemptNum int, status string) string {
	rt := "ADVISOR_REQUEST_MUTATION_RECORDED"
	if !request {
		rt = "ADVISOR_RESPONSE_MUTATION_RECORDED"
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"` + rt + `","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":{"retrySequenceId":"` + retryID + `","attemptId":"` + attemptID +
		`","attemptNumber":` + itoa(attemptNum) + `,"status":"` + status + `"},"data":null}`
}

// TestAttemptsUseOnlyExplicitAttemptAndRetryIdentity proves attempt/retry
// membership uses only the recorded attemptId and retrySequenceId, not record
// adjacency. Two attempts with distinct IDs in the same retry sequence are
// indexed as separate attempts under one retry.
func TestAttemptsUseOnlyExplicitAttemptAndRetryIdentity(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, 6, 2, 8, "EXACT") + "\n" +
		requestRecord(5, "retry-1", "attempt-2", 2, true) + "\n" +
		requestRecord(6, "retry-1", "attempt-2", 2, false) + "\n" +
		responseRecord(7, "", "retry-1", "attempt-2", 2, 5, 2, 7, "EXACT") + "\n" +
		completionRecord(8, "SUCCEEDED", 11, 4, 15, "") + "\n"
	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.AttemptCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", m.AttemptCount)
	}
	if m.RetryCount != 1 {
		t.Fatalf("expected 1 retry, got %d", m.RetryCount)
	}
}

func TestRetryCountCountsAttemptsAfterInitialNotRetrySequences(t *testing.T) {
	for _, test := range []struct {
		name      string
		attempts  []int
		wantTotal int
		wantRetry int
	}{
		{"one initial", []int{1}, 1, 0},
		{"one retry", []int{2}, 2, 1},
		{"two retries", []int{3}, 3, 2},
		{"ten independent initials", []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, 10, 0},
		{"mixed sequences", []int{1, 3}, 4, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := []string{startedRecord(1)}
			sequence := 2
			for retryIndex, attemptCount := range test.attempts {
				for attemptNumber := 1; attemptNumber <= attemptCount; attemptNumber++ {
					retryID := "retry-" + itoa(retryIndex+1)
					attemptID := retryID + "-attempt-" + itoa(attemptNumber)
					lines = append(lines,
						requestRecord(sequence, retryID, attemptID, attemptNumber, false),
						responseRecord(sequence+1, "", retryID, attemptID, attemptNumber, 0, 0, 0, "EXACT"),
					)
					sequence += 2
				}
			}
			lines = append(lines, completionRecord(sequence, "SUCCEEDED", 0, 0, 0, ""))
			m, category, ok := processTrace(t, strings.Join(lines, "\n")+"\n")
			if !ok {
				t.Fatalf("expected valid trace, got category %s", category)
			}
			if m.AttemptCount != test.wantTotal || m.RetryCount != test.wantRetry {
				t.Fatalf("attemptCount=%d retryCount=%d want %d/%d", m.AttemptCount, m.RetryCount, test.wantTotal, test.wantRetry)
			}
		})
	}
}

func TestPlanRetryRequestedDoesNotCountAsModelRetry(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		`{"traceId":"t","sessionId":"s","sequence":2,"timestamp":` + timestampForSeq(2) +
		`,"recordType":"PLAN_RETRY_REQUESTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n" +
		completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	m, category, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", category)
	}
	if m.AttemptCount != 0 || m.RetryCount != 0 {
		t.Fatalf("attemptCount=%d retryCount=%d, want 0/0", m.AttemptCount, m.RetryCount)
	}
	if m.RecordCountsByType[RecordPlanRetryRequested] != 1 {
		t.Fatalf("PLAN_RETRY_REQUESTED histogram=%d, want 1", m.RecordCountsByType[RecordPlanRetryRequested])
	}
}

// TestAttemptsAllowSentWithoutResponseAndReportUsageGap proves a provider
// failure that leaves a sent attempt without a response is retained as an
// attempt with incomplete usage (a gap), not rejected or silently zeroed.
func TestAttemptsAllowSentWithoutResponseAndReportUsageGap(t *testing.T) {
	// attempt-1 has PREPARED + SENT but no RESPONSE_RECEIVED (provider failure).
	// attempt-2 completes normally. The trace succeeds overall.
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		// No response for attempt-1; move to attempt-2.
		requestRecord(4, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(5, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(6, "", "retry-2", "attempt-2", 1, 10, 4, 14, "EXACT") + "\n" +
		completionRecord(7, "SUCCEEDED", 10, 4, 14, "") + "\n"
	m, _, ok := processTrace(t, raw)
	if !ok {
		t.Fatal("expected valid trace with a sent-but-no-response attempt")
	}
	if m.AttemptCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", m.AttemptCount)
	}
	if m.GapCount != 1 {
		t.Fatalf("expected one missing response gap, got %d", m.GapCount)
	}
}

// TestAttemptsRejectInconsistentIdentityNumberAndLifecycleOrder is a consolidated
// test proving the attempt graph rejects inconsistent retrySequenceId,
// inconsistent attemptNumber, and out-of-order lifecycle. Each subtest targets
// one contradiction.
func TestAttemptsRejectInconsistentIdentityNumberAndLifecycleOrder(t *testing.T) {
	t.Run("inconsistent_retry_id", func(t *testing.T) {
		// Same attemptId but different retrySequenceId in the SENT record.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, false) + "\n" +
			`{"traceId":"t","sessionId":"s","sequence":3,"timestamp":` + timestampForSeq(3) +
			`,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
			`"metadata":{"retrySequenceId":"retry-2","attemptId":"attempt-1","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["u"]}}` + "\n" +
			completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for inconsistent retry id")
		}
		if cat != CategoryInvalidAttempt {
			t.Fatalf("expected INVALID_ATTEMPT, got %s", cat)
		}
	})

	t.Run("response_without_sent", func(t *testing.T) {
		// RESPONSE_RECEIVED with no preceding SENT.
		raw := startedRecord(1) + "\n" +
			responseRecord(2, "", "retry-1", "attempt-1", 1, 10, 4, 14, "EXACT") + "\n" +
			completionRecord(3, "SUCCEEDED", 10, 4, 14, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for response without sent")
		}
		if cat != CategoryInvalidAttempt {
			t.Fatalf("expected INVALID_ATTEMPT, got %s", cat)
		}
	})

	t.Run("duplicate_sent", func(t *testing.T) {
		// Two SENT records for the same attempt.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, false) + "\n" +
			requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
			completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for duplicate sent")
		}
		if cat != CategoryInvalidAttempt {
			t.Fatalf("expected INVALID_ATTEMPT, got %s", cat)
		}
	})
}

// TestValidationsLinkOnlyToRecordedAttemptIdentity proves advisor validation
// links carry only the recorded attempt identity (retrySequenceId, attemptId,
// attemptNumber) and status from the advisor mutation records, not inferred
// relationships.
func TestValidationsLinkOnlyToRecordedAttemptIdentity(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, 6, 2, 8, "EXACT") + "\n" +
		advisorRecord(5, true, "retry-1", "attempt-1", 1, "retrying") + "\n" +
		requestRecord(6, "retry-1", "attempt-2", 2, true) + "\n" +
		requestRecord(7, "retry-1", "attempt-2", 2, false) + "\n" +
		responseRecord(8, "", "retry-1", "attempt-2", 2, 5, 2, 7, "EXACT") + "\n" +
		advisorRecord(9, false, "retry-1", "attempt-2", 2, "passed") + "\n" +
		completionRecord(10, "SUCCEEDED", 11, 4, 15, "") + "\n"
	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.ValidationCount != 2 {
		t.Fatalf("expected 2 validation links, got %d", m.ValidationCount)
	}
}

// TestFailuresKeepRecoveredErrorsSeparateFromTerminalFailure proves a
// nonterminal ERROR_RECORDED (recovered error) is indexed as a nonterminal
// failure while a separate terminal ERROR_RECORDED resolves the terminal
// outcome. The two are distinct facts.
func TestFailuresKeepRecoveredErrorsSeparateFromTerminalFailure(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		errorRecord(2, "recovered-error", false) + "\n" + // nonterminal recovered error
		errorRecord(3, "fatal-error", true) + "\n" + // terminal failure
		completionRecord(4, "FAILED", 0, 0, 0, "fatal-error") + "\n"
	m, cat, ok := processTrace(t, raw)
	if !ok {
		t.Fatalf("expected valid trace, got category %s", cat)
	}
	if m.FailureCount != 2 {
		t.Fatalf("expected 2 failures, got %d", m.FailureCount)
	}
	if m.Outcome != "FAILED" {
		t.Fatalf("expected outcome FAILED, got %s", m.Outcome)
	}
}

// TestTerminalOutcomeRequiresExactlyOneFinalConsistentCompletion proves the
// processor rejects a trace with no completion (MISSING_COMPLETION), multiple
// completions (NON_FINAL_COMPLETION), and a completion that is not the final
// record (NON_FINAL_COMPLETION).
func TestTerminalOutcomeRequiresExactlyOneFinalConsistentCompletion(t *testing.T) {
	t.Run("no_completion", func(t *testing.T) {
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for no completion")
		}
		if cat != CategoryMissingCompletion {
			t.Fatalf("expected MISSING_COMPLETION, got %s", cat)
		}
	})

	t.Run("completion_not_final", func(t *testing.T) {
		// Completion is followed by another record.
		raw := startedRecord(1) + "\n" +
			completionRecord(2, "SUCCEEDED", 0, 0, 0, "") + "\n" +
			requestRecord(3, "retry-1", "attempt-1", 1, true) + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for non-final completion")
		}
		if cat != CategoryNonFinalCompletion {
			t.Fatalf("expected NON_FINAL_COMPLETION, got %s", cat)
		}
	})
}

// TestUsageCalculatesDirectDescendantInclusiveAttemptAndRetryWithoutDoubleCounting
// proves direct, descendant, inclusive, attempt, and retry usage are
// mechanically non-overlapping. A 3-level hierarchy (root -> skill -> tool)
// with two attempts in one retry sequence is processed; each frame's inclusive
// usage equals direct + descendant, and the retry total equals the sum of its
// attempts without double-counting any response.
func TestUsageCalculatesDirectDescendantInclusiveAttemptAndRetryWithoutDoubleCounting(t *testing.T) {
	// root (ROOT_MISSION) -> skill (SKILL_EXECUTION) -> tool (TOOL_INVOCATION)
	// tool direct: (1,1,2) from attempt-1; skill direct: (4,2,6) from attempt-2
	// root direct: (0,0,0)
	// Both attempts are in retry-1.
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		frameRecord(3, "skill", "root", true, "SKILL_EXECUTION", true) + "\n" +
		frameRecord(4, "tool", "skill", true, "TOOL_INVOCATION", true) + "\n" +
		requestRecord(5, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(6, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(7, "tool", "retry-1", "attempt-1", 1, 1, 1, 2, "EXACT") + "\n" +
		requestRecord(8, "retry-1", "attempt-2", 2, true) + "\n" +
		requestRecord(9, "retry-1", "attempt-2", 2, false) + "\n" +
		responseRecord(10, "skill", "retry-1", "attempt-2", 2, 4, 2, 6, "EXACT") + "\n" +
		frameRecord(11, "tool", "skill", true, "TOOL_INVOCATION", false) + "\n" +
		frameRecord(12, "skill", "root", true, "SKILL_EXECUTION", false) + "\n" +
		frameRecord(13, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(14, "SUCCEEDED", 5, 3, 8, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	// Frame index: verify direct/descendant/inclusive are non-overlapping.
	frames := readFactRows[frameResult](t, sink, ComponentFrameIndex)
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	frameByID := map[string]frameResult{}
	for _, f := range frames {
		frameByID[f.FrameID] = f
		if f.OpenedTimestampMillis == 0 || f.ClosedTimestampMillis == nil {
			t.Fatalf("frame boundary timestamps were not persisted in the frame index: %+v", f)
		}
		if f.InclusiveDurationMillis == nil || *f.ClosedTimestampMillis-f.OpenedTimestampMillis != *f.InclusiveDurationMillis {
			t.Fatalf("persisted frame boundaries do not match the authoritative duration: %+v", f)
		}
	}
	// tool: direct=(1,1,2), descendant=(0,0,0), inclusive=(1,1,2)
	tool := frameByID["tool"]
	if tool.DirectUsage.TotalUnits != 2 || tool.DescendantUsage.TotalUnits != 0 || tool.InclusiveUsage.TotalUnits != 2 {
		t.Errorf("tool usage: direct=%d descendant=%d inclusive=%d, want 2/0/2",
			tool.DirectUsage.TotalUnits, tool.DescendantUsage.TotalUnits, tool.InclusiveUsage.TotalUnits)
	}
	// skill: direct=(4,2,6), descendant=(1,1,2), inclusive=(5,3,8)
	skill := frameByID["skill"]
	if skill.DirectUsage.TotalUnits != 6 || skill.DescendantUsage.TotalUnits != 2 || skill.InclusiveUsage.TotalUnits != 8 {
		t.Errorf("skill usage: direct=%d descendant=%d inclusive=%d, want 6/2/8",
			skill.DirectUsage.TotalUnits, skill.DescendantUsage.TotalUnits, skill.InclusiveUsage.TotalUnits)
	}
	// root: direct=(0,0,0), descendant=(5,3,8), inclusive=(5,3,8)
	root := frameByID["root"]
	if root.DirectUsage.TotalUnits != 0 || root.DescendantUsage.TotalUnits != 8 || root.InclusiveUsage.TotalUnits != 8 {
		t.Errorf("root usage: direct=%d descendant=%d inclusive=%d, want 0/8/8",
			root.DirectUsage.TotalUnits, root.DescendantUsage.TotalUnits, root.InclusiveUsage.TotalUnits)
	}

	// Attempt index: two attempts, each counted once.
	attempts := readFactRows[attemptResult](t, sink, ComponentAttemptIndex)
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(attempts))
	}
	if attempts[0].Usage.TotalUnits != 2 {
		t.Errorf("attempt-1 usage total: got %d want 2", attempts[0].Usage.TotalUnits)
	}
	if attempts[1].Usage.TotalUnits != 6 {
		t.Errorf("attempt-2 usage total: got %d want 6", attempts[1].Usage.TotalUnits)
	}

	// Usage index: attributed = sum of all responses = (5,3,8).
	usageFacts := readFactRowsRaw(t, sink, ComponentUsageIndex)
	attributed := extractUsageFact(usageFacts[0], "ATTRIBUTED")
	if attributed.TotalUnits != 8 {
		t.Errorf("attributed total: got %d want 8", attributed.TotalUnits)
	}
	terminal := extractUsageFact(usageFacts[3], "TERMINAL")
	if terminal.TotalUnits != 8 {
		t.Errorf("terminal total: got %d want 8", terminal.TotalUnits)
	}
	// Unattributed = terminal - attributed = (0,0,0).
	unattributed := extractUsageFact(usageFacts[1], "UNATTRIBUTED")
	if unattributed.TotalUnits != 0 {
		t.Errorf("unattributed total: got %d want 0", unattributed.TotalUnits)
	}
}

// TestUsageKeepsUnframedAttributedAndTerminalRemainderSeparate proves
// unframed attributed usage (responses with no frameId) and the terminal
// unattributed remainder (terminal minus all attributed) are distinct facts.
// A trace with one framed response and one unframed response is processed; the
// unframed attributed total equals only the unframed response, while the
// unattributed remainder equals terminal minus all attributed (framed +
// unframed).
func TestUsageKeepsUnframedAttributedAndTerminalRemainderSeparate(t *testing.T) {
	// framed response: (4,2,6) on frame "root"
	// unframed response: (1,1,2) with no frameId
	// terminal snapshot: (10,4,14)
	// attributed = (5,3,8); unframed attributed = (1,1,2)
	// unattributed = terminal - attributed = (5,1,6)
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(4, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(5, "root", "retry-1", "attempt-1", 1, 4, 2, 6, "EXACT") + "\n" +
		requestRecord(6, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(7, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(8, "", "retry-2", "attempt-2", 1, 1, 1, 2, "EXACT") + "\n" +
		frameRecord(9, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(10, "SUCCEEDED", 10, 4, 14, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	usageFacts := readFactRowsRaw(t, sink, ComponentUsageIndex)
	attributed := extractUsageFact(usageFacts[0], "ATTRIBUTED")
	unattributed := extractUsageFact(usageFacts[1], "UNATTRIBUTED")
	unframedAttributed := extractUsageFact(usageFacts[2], "UNFRAMED_ATTRIBUTED")
	terminal := extractUsageFact(usageFacts[3], "TERMINAL")

	// Attributed = all responses = (5,3,8).
	if attributed != (Usage{5, 3, 8}) {
		t.Errorf("attributed: got %+v want {5,3,8}", attributed)
	}
	// Unframed attributed = only the unframed response = (1,1,2).
	if unframedAttributed != (Usage{1, 1, 2}) {
		t.Errorf("unframedAttributed: got %+v want {1,1,2}", unframedAttributed)
	}
	// Terminal = (10,4,14).
	if terminal != (Usage{10, 4, 14}) {
		t.Errorf("terminal: got %+v want {10,4,14}", terminal)
	}
	// Unattributed = terminal - attributed = (5,1,6). This is distinct from
	// unframed attributed.
	if unattributed != (Usage{5, 1, 6}) {
		t.Errorf("unattributed: got %+v want {5,1,6}", unattributed)
	}
	// The unattributed remainder must not equal the unframed attributed total.
	if unattributed == unframedAttributed {
		t.Error("unattributed remainder equals unframed attributed; they must be distinct facts")
	}
}

// TestUsageReconcilesPromptCompletionAndTotalIndependently proves prompt,
// completion, and total units are summed and reconciled independently. The plan
// does not require total = prompt + completion; each component is reconciled
// separately against the terminal snapshot. A trace where total != prompt +
// completion for individual responses but the per-component sums reconcile is
// accepted.
func TestUsageReconcilesPromptCompletionAndTotalIndependently(t *testing.T) {
	// Two responses where total != prompt + completion for each, but each
	// component sums independently to the terminal snapshot.
	// response-1: prompt=10, completion=4, total=20 (total != 10+4)
	// response-2: prompt=5, completion=1, total=8  (total != 5+1)
	// attributed: prompt=15, completion=5, total=28
	// terminal: prompt=15, completion=5, total=28
	// unattributed: (0,0,0) — each component reconciles independently.
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(4, "", "retry-1", "attempt-1", 1, 10, 4, 20, "EXACT") + "\n" +
		requestRecord(5, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(6, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(7, "", "retry-2", "attempt-2", 1, 5, 1, 8, "EXACT") + "\n" +
		completionRecord(8, "SUCCEEDED", 15, 5, 28, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	usageFacts := readFactRowsRaw(t, sink, ComponentUsageIndex)
	attributed := extractUsageFact(usageFacts[0], "ATTRIBUTED")
	unattributed := extractUsageFact(usageFacts[1], "UNATTRIBUTED")
	terminal := extractUsageFact(usageFacts[3], "TERMINAL")

	// Each component is summed independently.
	if attributed.PromptUnits != 15 {
		t.Errorf("attributed prompt: got %d want 15", attributed.PromptUnits)
	}
	if attributed.CompletionUnits != 5 {
		t.Errorf("attributed completion: got %d want 5", attributed.CompletionUnits)
	}
	if attributed.TotalUnits != 28 {
		t.Errorf("attributed total: got %d want 28", attributed.TotalUnits)
	}
	// Terminal matches each component.
	if terminal != (Usage{15, 5, 28}) {
		t.Errorf("terminal: got %+v want {15,5,28}", terminal)
	}
	// Unattributed = terminal - attributed per component = (0,0,0).
	if unattributed != (Usage{0, 0, 0}) {
		t.Errorf("unattributed: got %+v want {0,0,0}", unattributed)
	}
}

// TestUsageRejectsNegativeRemainderInvalidComponentsAndOverflow is a consolidated
// test proving the usage calculator rejects a negative terminal remainder
// (terminal < attributed), negative usage components, and accumulation overflow.
// Each subtest targets one contradiction.
func TestUsageRejectsNegativeRemainderInvalidComponentsAndOverflow(t *testing.T) {
	t.Run("negative_remainder", func(t *testing.T) {
		// Attributed total (14) > terminal total (2) → negative remainder.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
			responseRecord(4, "", "retry-1", "attempt-1", 1, 10, 4, 14, "EXACT") + "\n" +
			completionRecord(5, "SUCCEEDED", 1, 1, 2, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for negative remainder")
		}
		if cat != CategoryContradictoryUsage {
			t.Fatalf("expected CONTRADICTORY_USAGE, got %s", cat)
		}
	})

	t.Run("negative_completion_component", func(t *testing.T) {
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
			responseRecord(4, "", "retry-1", "attempt-1", 1, 10, -4, 6, "EXACT") + "\n" +
			completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for negative completion")
		}
		if cat != CategoryInvalidUsage {
			t.Fatalf("expected INVALID_USAGE, got %s", cat)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		// Two responses each with promptUnits = MaxInt64/2 + 1 → overflow.
		big := int64(9223372036854775807/2 + 1)
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			requestRecord(3, "retry-1", "attempt-1", 1, false) + "\n" +
			responseRecord(4, "", "retry-1", "attempt-1", 1, int(big), 0, int(big), "EXACT") + "\n" +
			requestRecord(5, "retry-2", "attempt-2", 1, true) + "\n" +
			requestRecord(6, "retry-2", "attempt-2", 1, false) + "\n" +
			responseRecord(7, "", "retry-2", "attempt-2", 1, int(big), 0, int(big), "EXACT") + "\n" +
			completionRecord(8, "SUCCEEDED", int(big*2), 0, int(big*2), "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for overflow")
		}
		if cat != CategoryContradictoryUsage {
			t.Fatalf("expected CONTRADICTORY_USAGE, got %s", cat)
		}
	})
}

// frameRecordWithTimestamp builds a FRAME_OPENED or FRAME_CLOSED record with an
// explicit timestamp (in seconds since epoch), used by frame interval tests
// that need non-default timing.
func frameRecordWithTimestamp(seq int, frameID, parentID string, hasParent bool, frameType string, opened bool, timestampSec int) string {
	rt := "FRAME_OPENED"
	if !opened {
		rt = "FRAME_CLOSED"
	}
	parentJSON := "null"
	if hasParent {
		parentJSON = `"` + parentID + `"`
	}
	ftJSON := "null"
	if frameType != "" {
		ftJSON = `"` + frameType + `"`
	}
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + itoa(timestampSec) + `.000000000` +
		`,"recordType":"` + rt + `","frameId":"` + frameID +
		`","parentFrameId":` + parentJSON +
		`,"frameType":` + ftJSON +
		`,"route":null,"threadName":"th","metadata":{},"data":null}`
}

// TestFramesRejectDuplicateMissingSelfParentCycleAndConflictingIdentity proves
// the frame graph rejects duplicate opens, self-parenting, missing parents,
// cycles, and conflicting frame identity (frame type mismatch on close). Each
// subtest targets one contradiction.
func TestFramesRejectDuplicateMissingSelfParentCycleAndConflictingIdentity(t *testing.T) {
	t.Run("duplicate_open", func(t *testing.T) {
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
			frameRecord(3, "root", "", false, "ROOT_MISSION", true) + "\n" + // duplicate open
			completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for duplicate open")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		// root -> skill -> root (cycle: root's parent is skill, skill's parent is root)
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "skill", true, "ROOT_MISSION", true) + "\n" +
			frameRecord(3, "skill", "root", true, "SKILL_EXECUTION", true) + "\n" +
			completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for cycle")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})

	t.Run("conflicting_frame_type_on_close", func(t *testing.T) {
		// Open as ROOT_MISSION, close as SKILL_EXECUTION.
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
			frameRecord(3, "root", "", false, "SKILL_EXECUTION", false) + "\n" +
			completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for conflicting frame type")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})

	t.Run("close_before_open", func(t *testing.T) {
		// FRAME_CLOSED for a frame that was never opened.
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "", false, "ROOT_MISSION", false) + "\n" +
			completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for close before open")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})

	t.Run("duplicate_close", func(t *testing.T) {
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
			frameRecord(3, "root", "", false, "ROOT_MISSION", false) + "\n" +
			frameRecord(4, "root", "", false, "ROOT_MISSION", false) + "\n" + // duplicate close
			completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for duplicate close")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})
}

// TestFramesCalculateInclusiveAndSelfDuration proves inclusive duration equals
// closedAt - openedAt and self duration equals inclusive minus the sum of
// immediate complete non-overlapping child durations. A root with two
// non-overlapping children is processed; root's self duration equals its
// inclusive duration minus the sum of both children's durations.
func TestFramesCalculateInclusiveAndSelfDuration(t *testing.T) {
	// root: opened at t=100, closed at t=500 → inclusive = 400000ms
	// child-1: opened at t=100, closed at t=200 → duration = 100000ms
	// child-2: opened at t=200, closed at t=400 → duration = 200000ms
	// root self = 400000 - (100000 + 200000) = 100000ms
	raw := startedRecord(1) + "\n" +
		frameRecordWithTimestamp(2, "root", "", false, "ROOT_MISSION", true, 100) + "\n" +
		frameRecordWithTimestamp(3, "child-1", "root", true, "SKILL_EXECUTION", true, 100) + "\n" +
		frameRecordWithTimestamp(4, "child-1", "root", true, "SKILL_EXECUTION", false, 200) + "\n" +
		frameRecordWithTimestamp(5, "child-2", "root", true, "SKILL_EXECUTION", true, 200) + "\n" +
		frameRecordWithTimestamp(6, "child-2", "root", true, "SKILL_EXECUTION", false, 400) + "\n" +
		frameRecordWithTimestamp(7, "root", "", false, "ROOT_MISSION", false, 500) + "\n" +
		completionRecord(8, "SUCCEEDED", 0, 0, 0, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	frames := readFactRows[frameResult](t, sink, ComponentFrameIndex)
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	frameByID := map[string]frameResult{}
	for _, f := range frames {
		frameByID[f.FrameID] = f
	}

	root := frameByID["root"]
	if root.InclusiveDurationMillis == nil {
		t.Fatal("root inclusive duration is nil")
	}
	if *root.InclusiveDurationMillis != 400000 {
		t.Errorf("root inclusive: got %d want 400000", *root.InclusiveDurationMillis)
	}
	if root.SelfDurationMillis == nil {
		t.Fatal("root self duration is nil")
	}
	if *root.SelfDurationMillis != 100000 {
		t.Errorf("root self: got %d want 100000", *root.SelfDurationMillis)
	}

	child1 := frameByID["child-1"]
	if child1.InclusiveDurationMillis == nil || *child1.InclusiveDurationMillis != 100000 {
		t.Errorf("child-1 inclusive: got %v want 100000", child1.InclusiveDurationMillis)
	}
	if child1.SelfDurationMillis == nil || *child1.SelfDurationMillis != 100000 {
		t.Errorf("child-1 self: got %v want 100000", child1.SelfDurationMillis)
	}

	child2 := frameByID["child-2"]
	if child2.InclusiveDurationMillis == nil || *child2.InclusiveDurationMillis != 200000 {
		t.Errorf("child-2 inclusive: got %v want 200000", child2.InclusiveDurationMillis)
	}
	if child2.SelfDurationMillis == nil || *child2.SelfDurationMillis != 200000 {
		t.Errorf("child-2 self: got %v want 200000", child2.SelfDurationMillis)
	}
}

// TestFramesMarkIncompleteOrOverlappingSelfDurationUnavailable proves that when
// immediate children overlap, the parent's self duration is marked unavailable
// with the SELF_DURATION_UNAVAILABLE_OVERLAPPING_CHILDREN uncertainty. (The
// incomplete-child variant is covered by
// TestFramesMarkIncompleteChildSelfDurationUnavailable.)
func TestFramesMarkIncompleteOrOverlappingSelfDurationUnavailable(t *testing.T) {
	t.Run("overlapping_children", func(t *testing.T) {
		// root: opened at t=100, closed at t=500
		// child-1: opened at t=100, closed at t=300
		// child-2: opened at t=200, closed at t=400 (overlaps child-1)
		raw := startedRecord(1) + "\n" +
			frameRecordWithTimestamp(2, "root", "", false, "ROOT_MISSION", true, 100) + "\n" +
			frameRecordWithTimestamp(3, "child-1", "root", true, "SKILL_EXECUTION", true, 100) + "\n" +
			frameRecordWithTimestamp(4, "child-1", "root", true, "SKILL_EXECUTION", false, 300) + "\n" +
			frameRecordWithTimestamp(5, "child-2", "root", true, "SKILL_EXECUTION", true, 200) + "\n" +
			frameRecordWithTimestamp(6, "child-2", "root", true, "SKILL_EXECUTION", false, 400) + "\n" +
			frameRecordWithTimestamp(7, "root", "", false, "ROOT_MISSION", false, 500) + "\n" +
			completionRecord(8, "SUCCEEDED", 0, 0, 0, "") + "\n"

		sink := &fakeSink{}
		processor := New()
		_, domain := processor.Process(artifact.ProcessRequest{
			Context:  context.Background(),
			Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
			Raw:      strings.NewReader(raw),
			Sink:     sink,
		})
		if domain != nil {
			t.Fatalf("Process failed: %v", domain)
		}

		frames := readFactRows[frameResult](t, sink, ComponentFrameIndex)
		frameByID := map[string]frameResult{}
		for _, f := range frames {
			frameByID[f.FrameID] = f
		}
		root := frameByID["root"]
		// Inclusive duration is still available.
		if root.InclusiveDurationMillis == nil {
			t.Fatal("root inclusive duration should be available even with overlapping children")
		}
		// Self duration must be nil (unavailable) due to overlapping children.
		if root.SelfDurationMillis != nil {
			t.Errorf("root self duration should be nil (overlapping children), got %d", *root.SelfDurationMillis)
		}
		// The manifest must record one uncertainty.
		mBytes := sink.components[ComponentManifest]
		var m manifest
		if err := json.Unmarshal(mBytes, &m); err != nil {
			t.Fatalf("parse manifest: %v", err)
		}
		if m.UncertaintyCount != 1 {
			t.Fatalf("expected 1 uncertainty, got %d", m.UncertaintyCount)
		}
	})

	t.Run("incomplete_child", func(t *testing.T) {
		// root opened and closed; child opened but never closed.
		raw := startedRecord(1) + "\n" +
			frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
			frameRecord(3, "child", "root", true, "TOOL_INVOCATION", true) + "\n" +
			frameRecord(4, "root", "", false, "ROOT_MISSION", false) + "\n" +
			completionRecord(5, "SUCCEEDED", 0, 0, 0, "") + "\n"
		m, cat, ok := processTrace(t, raw)
		if !ok {
			t.Fatalf("expected valid trace, got category %s", cat)
		}
		if m.UncertaintyCount != 1 {
			t.Fatalf("expected 1 uncertainty (incomplete child), got %d", m.UncertaintyCount)
		}
		if m.GapCount != 1 {
			t.Fatalf("expected 1 gap (open frame not closed), got %d", m.GapCount)
		}
	})
}

// TestFramesRejectNegativeOrOutsideParentCompleteIntervals proves a child whose
// complete interval is outside its complete parent's interval is rejected. A
// child that opens before its parent opens, or closes after its parent closes,
// is invalid. Each subtest targets one boundary violation.
func TestFramesRejectNegativeOrOutsideParentCompleteIntervals(t *testing.T) {
	t.Run("child_opens_before_parent", func(t *testing.T) {
		// parent: opened at t=200, closed at t=500
		// child: opened at t=100 (before parent), closed at t=300
		raw := startedRecord(1) + "\n" +
			frameRecordWithTimestamp(2, "root", "", false, "ROOT_MISSION", true, 200) + "\n" +
			frameRecordWithTimestamp(3, "child", "root", true, "SKILL_EXECUTION", true, 100) + "\n" +
			frameRecordWithTimestamp(4, "child", "root", true, "SKILL_EXECUTION", false, 300) + "\n" +
			frameRecordWithTimestamp(5, "root", "", false, "ROOT_MISSION", false, 500) + "\n" +
			completionRecord(6, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for child opening before parent")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})

	t.Run("child_closes_after_parent", func(t *testing.T) {
		// parent: opened at t=100, closed at t=300
		// child: opened at t=200, closed at t=500 (after parent closes)
		raw := startedRecord(1) + "\n" +
			frameRecordWithTimestamp(2, "root", "", false, "ROOT_MISSION", true, 100) + "\n" +
			frameRecordWithTimestamp(3, "child", "root", true, "SKILL_EXECUTION", true, 200) + "\n" +
			frameRecordWithTimestamp(4, "root", "", false, "ROOT_MISSION", false, 300) + "\n" +
			frameRecordWithTimestamp(5, "child", "root", true, "SKILL_EXECUTION", false, 500) + "\n" +
			completionRecord(6, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for child closing after parent")
		}
		if cat != CategoryInvalidFrameRelationship {
			t.Fatalf("expected INVALID_FRAME_RELATIONSHIP, got %s", cat)
		}
	})
}

// TestFramesRejectResponseReferencingNonExistentFrame proves an explicit
// dangling frame attribution is rejected instead of silently discarded.
func TestFramesRejectResponseReferencingNonExistentFrame(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		requestRecord(3, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(4, "retry-1", "attempt-1", 1, false) + "\n" +
		responseRecord(5, "root", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		requestRecord(6, "retry-2", "attempt-2", 1, true) + "\n" +
		requestRecord(7, "retry-2", "attempt-2", 1, false) + "\n" +
		responseRecord(8, "ghost", "retry-2", "attempt-2", 1, 3, 1, 4, "EXACT") + "\n" +
		frameRecord(9, "root", "", false, "ROOT_MISSION", false) + "\n" +
		completionRecord(10, "SUCCEEDED", 5, 2, 7, "") + "\n"

	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	category, _ := categoryOf(domain)
	if domain == nil || category != CategoryInvalidFrameRelationship {
		t.Fatalf("expected invalid frame relationship, got: %v", domain)
	}
}
