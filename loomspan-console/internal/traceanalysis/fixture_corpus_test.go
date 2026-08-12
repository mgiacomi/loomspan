package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// fixtureRoot locates the loomspan-console-fixtures directory without copying
// fixtures into the Go module. It mirrors the Java corpus test's resolution.
func fixtureRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	candidates := []string{
		filepath.Join(cwd, "..", "..", "loomspan-console-fixtures"),
		filepath.Join(cwd, "..", "..", "..", "loomspan-console-fixtures"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(c, "traces")); err == nil && info.IsDir() {
			return c
		}
	}
	t.Fatalf("loomspan-console-fixtures not found relative to %s", cwd)
	return ""
}

// expectedFile is the JSON shape of one committed expected result.
type expectedFile struct {
	Case                    string            `json:"case"`
	Valid                   bool              `json:"valid"`
	TraceID                 string            `json:"traceId,omitempty"`
	SessionID               string            `json:"sessionId,omitempty"`
	Outcome                 string            `json:"outcome,omitempty"`
	TerminalFailureID       *string           `json:"terminalFailureId,omitempty"`
	ConfiguredLimits        *ConfiguredLimits `json:"configuredLimits,omitempty"`
	AttributedUsage         json.RawMessage   `json:"attributedUsage,omitempty"`
	TerminalUsage           json.RawMessage   `json:"terminalUsage,omitempty"`
	UnattributedUsage       json.RawMessage   `json:"unattributedUsage,omitempty"`
	UsageComplete           bool              `json:"usageComplete,omitempty"`
	Attempts                json.RawMessage   `json:"attempts,omitempty"`
	Retries                 json.RawMessage   `json:"retries,omitempty"`
	ValidationLinks         json.RawMessage   `json:"validationLinks,omitempty"`
	Frames                  json.RawMessage   `json:"frames,omitempty"`
	UnframedAttributedUsage json.RawMessage   `json:"unframedAttributedUsage,omitempty"`
	Payloads                json.RawMessage   `json:"payloads,omitempty"`
	Gaps                    json.RawMessage   `json:"gaps,omitempty"`
	Uncertainties           json.RawMessage   `json:"uncertainties,omitempty"`
	ErrorCategory           string            `json:"errorCategory,omitempty"`
}

// TestFixtureCorpusMatchesJavaExpectedSemantics processes every Java fixture in
// place and compares the neutral semantic result or exact invalidity category
// against the committed expected file.
func TestFixtureCorpusMatchesJavaExpectedSemantics(t *testing.T) {
	root := fixtureRoot(t)
	traceDir := filepath.Join(root, "traces")
	expectedDir := filepath.Join(root, "expected")

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("read traces dir: %v", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ndjson") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".ndjson")
		t.Run(name, func(t *testing.T) {
			tracePath := filepath.Join(traceDir, entry.Name())
			expectedPath := filepath.Join(expectedDir, name+".json")

			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("read expected %s: %v", name, err)
			}
			var expected expectedFile
			if err := json.Unmarshal(expectedBytes, &expected); err != nil {
				t.Fatalf("parse expected %s: %v", name, err)
			}

			traceBytes, err := os.ReadFile(tracePath)
			if err != nil {
				t.Fatalf("read trace %s: %v", name, err)
			}

			sink := &fakeSink{}
			processor := New()
			_, domain := processor.Process(artifact.ProcessRequest{
				Context: context.Background(),
				Metadata: artifact.TraceMetadata{
					TraceID:   expected.TraceID,
					SessionID: expected.SessionID,
					Outcome:   expected.Outcome,
				},
				Raw:  bytesReader(traceBytes),
				Sink: sink,
			})

			if !expected.Valid {
				if domain == nil {
					t.Fatalf("expected invalid artifact for %s, got success", name)
				}
				if domain.Code != consolecore.CodeInvalidArtifact {
					t.Fatalf("expected INVALID_ARTIFACT for %s, got %v", name, domain.Code)
				}
				cat, ok := categoryOf(domain)
				if !ok {
					t.Fatalf("expected invalidity category for %s, got none", name)
				}
				if string(cat) != expected.ErrorCategory {
					t.Fatalf("expected category %s for %s, got %s", expected.ErrorCategory, name, cat)
				}
				return
			}

			if domain != nil {
				t.Fatalf("expected valid artifact for %s, got error: %v (category=%v)", name, domain, categoryOfSafe(domain))
			}
			// Valid cases: build the full analysisResult from the manifest and
			// fact indexes, then compare against the committed expected file.
			// This catches calculation regressions that preserve identity/outcome.
			result := buildAnalysisResultFromSink(t, sink)
			compareAnalysisResult(t, name, result, expected)
		})
	}
}

func TestToolLifecycleFixturesExposeOneCanonicalStartAndTerminalRecord(t *testing.T) {
	root := fixtureRoot(t)
	cases := []struct {
		name         string
		terminalType string
		outcome      string
	}{
		{name: "planned-tool-success", terminalType: string(RecordToolCallCompleted), outcome: "SUCCEEDED"},
		{name: "unplanned-tool-failure", terminalType: string(RecordToolCallFailed), outcome: "FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "traces", tc.name+".ndjson"))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			sink := &fakeSink{}
			_, domain := New().Process(artifact.ProcessRequest{
				Context: context.Background(),
				Metadata: artifact.TraceMetadata{
					TraceID: "trace-" + tc.name, SessionID: "session-" + tc.name, Outcome: tc.outcome,
				},
				Raw: bytesReader(raw), Sink: sink,
			})
			if domain != nil {
				t.Fatalf("process fixture: %v", domain)
			}

			lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
			index := sink.components[artifact.ComponentName(ComponentRecordIndex)]
			if got, want := len(index)/recordIndexRowWidth, len(lines); got != want {
				t.Fatalf("record index reachability: got %d rows, want %d", got, want)
			}

			var starts, terminals []map[string]any
			for position, line := range lines {
				row := readRecordIndexRow(index[position*recordIndexRowWidth : (position+1)*recordIndexRowWidth])
				var record map[string]any
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatalf("parse indexed record %d: %v", position, err)
				}
				if row.Sequence != int64(record["sequence"].(float64)) {
					t.Fatalf("indexed sequence %d did not match record", row.Sequence)
				}
				switch record["recordType"] {
				case string(RecordToolCallStarted):
					starts = append(starts, record)
				case tc.terminalType:
					terminals = append(terminals, record)
				}
			}
			if len(starts) != 1 || len(terminals) != 1 {
				t.Fatalf("got %d starts and %d %s records", len(starts), len(terminals), tc.terminalType)
			}
			if starts[0]["frameId"] != terminals[0]["frameId"] {
				t.Fatalf("start and terminal frame identity differed")
			}
			if starts[0]["sequence"].(float64) >= terminals[0]["sequence"].(float64) {
				t.Fatalf("terminal did not follow start")
			}
		})
	}
}

// buildAnalysisResultFromSink reads the manifest and every fact index from the
// fake sink and assembles a complete analysisResult for comparison against the
// committed expected file.
func buildAnalysisResultFromSink(t *testing.T, sink *fakeSink) analysisResult {
	t.Helper()
	manifestBytes, ok := sink.components[ComponentManifest]
	if !ok {
		t.Fatal("expected manifest component")
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	result := analysisResult{
		Valid:     true,
		TraceID:   m.TraceID,
		SessionID: m.SessionID,
		Outcome:   m.Outcome,
	}
	result.TerminalFailureID = m.TerminalFailureID
	result.ConfiguredLimits = m.ConfiguredLimits

	// Usage index: ATTRIBUTED, UNATTRIBUTED, UNFRAMED_ATTRIBUTED, TERMINAL.
	usageFacts := readFactRowsRaw(t, sink, ComponentUsageIndex)
	if len(usageFacts) >= 4 {
		result.AttributedUsage = extractUsageFact(usageFacts[0], "ATTRIBUTED")
		result.UnattributedUsage = extractUsageFact(usageFacts[1], "UNATTRIBUTED")
		result.UnframedAttributedUsage = extractUsageFact(usageFacts[2], "UNFRAMED_ATTRIBUTED")
		result.TerminalUsage = extractUsageFact(usageFacts[3], "TERMINAL")
	}

	// Attempt index.
	result.Attempts = readFactRows[attemptResult](t, sink, ComponentAttemptIndex)

	// Retry results are not written as a separate index; derive from attempts.
	result.Retries = buildRetryResultsFromAttempts(result.Attempts)

	// Validation links.
	result.ValidationLinks = readFactRows[validationLink](t, sink, ComponentValidationIdx)

	// Frames.
	result.Frames = readFactRows[frameResult](t, sink, ComponentFrameIndex)

	// Payloads.
	result.Payloads = readFactRows[payloadDescriptor](t, sink, ComponentPayloadIndex)

	// Gaps.
	result.Gaps = readFactRows[gapResult](t, sink, ComponentGapIndex)

	// UsageComplete: true unless any attempt has incomplete usage.
	result.UsageComplete = true
	for _, a := range result.Attempts {
		if !a.UsageComplete {
			result.UsageComplete = false
			break
		}
	}

	return result
}

// readFactRows reads length-prefixed JSON rows from a sink component and
// unmarshals each into the target type.
func readFactRows[T any](t *testing.T, sink *fakeSink, name component) []T {
	t.Helper()
	raw, ok := sink.components[artifact.ComponentName(name)]
	if !ok {
		return nil
	}
	r := bytes.NewReader(raw)
	var out []T
	for r.Len() > 0 {
		row, err := readLengthPrefixed(r)
		if err != nil {
			t.Fatalf("read fact row from %s: %v", name, err)
		}
		var v T
		if err := json.Unmarshal(row, &v); err != nil {
			t.Fatalf("parse fact row from %s: %v", name, err)
		}
		out = append(out, v)
	}
	return out
}

// readFactRowsRaw returns raw length-prefixed rows as []map[string]any for
// usage facts that use map[string]any.
func readFactRowsRaw(t *testing.T, sink *fakeSink, name component) []map[string]any {
	t.Helper()
	raw, ok := sink.components[artifact.ComponentName(name)]
	if !ok {
		return nil
	}
	r := bytes.NewReader(raw)
	var out []map[string]any
	for r.Len() > 0 {
		row, err := readLengthPrefixed(r)
		if err != nil {
			t.Fatalf("read fact row from %s: %v", name, err)
		}
		var v map[string]any
		if err := json.Unmarshal(row, &v); err != nil {
			t.Fatalf("parse fact row from %s: %v", name, err)
		}
		out = append(out, v)
	}
	return out
}

// extractUsageFact extracts a Usage from a usage fact map.
func extractUsageFact(fact map[string]any, kind string) Usage {
	return Usage{
		PromptUnits:     jsonInt64(fact["promptUnits"]),
		CompletionUnits: jsonInt64(fact["completionUnits"]),
		TotalUnits:      jsonInt64(fact["totalUnits"]),
	}
}

// jsonInt64 converts a JSON-decoded number to int64.
func jsonInt64(v any) int64 {
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return int64(f)
}

// buildRetryResultsFromAttempts aggregates attempt results into retry results
// matching the processor's buildAttemptResults logic.
func buildRetryResultsFromAttempts(attempts []attemptResult) []retryResult {
	retryUsage := map[string]Usage{}
	retryComplete := map[string]bool{}
	retryOrder := []string{}
	for _, a := range attempts {
		if _, seen := retryUsage[a.RetrySequenceID]; !seen {
			retryOrder = append(retryOrder, a.RetrySequenceID)
			retryComplete[a.RetrySequenceID] = true
		}
		var ok bool
		retryUsage[a.RetrySequenceID], ok = retryUsage[a.RetrySequenceID].plus(a.Usage)
		if !ok {
			panic("overflow in test helper buildRetryResultsFromAttempts")
		}
		retryComplete[a.RetrySequenceID] = retryComplete[a.RetrySequenceID] && a.UsageComplete
	}
	retries := make([]retryResult, 0, len(retryOrder))
	for _, rid := range retryOrder {
		retries = append(retries, retryResult{
			RetrySequenceID: rid,
			Usage:           retryUsage[rid],
			UsageComplete:   retryComplete[rid],
		})
	}
	return retries
}

// compareAnalysisResult compares the processor's calculated result against the
// committed expected file field by field.
func compareAnalysisResult(t *testing.T, name string, result analysisResult, expected expectedFile) {
	t.Helper()
	if result.TraceID != expected.TraceID {
		t.Errorf("traceId mismatch for %s: got %s want %s", name, result.TraceID, expected.TraceID)
	}
	if result.SessionID != expected.SessionID {
		t.Errorf("sessionId mismatch for %s: got %s want %s", name, result.SessionID, expected.SessionID)
	}
	if result.Outcome != expected.Outcome {
		t.Errorf("outcome mismatch for %s: got %s want %s", name, result.Outcome, expected.Outcome)
	}
	if (result.TerminalFailureID == nil) != (expected.TerminalFailureID == nil) {
		t.Errorf("terminalFailureId presence mismatch for %s: got %v want %v", name, result.TerminalFailureID, expected.TerminalFailureID)
	}
	if result.TerminalFailureID != nil && expected.TerminalFailureID != nil && *result.TerminalFailureID != *expected.TerminalFailureID {
		t.Errorf("terminalFailureId mismatch for %s: got %s want %s", name, *result.TerminalFailureID, *expected.TerminalFailureID)
	}
	if (result.ConfiguredLimits == nil) != (expected.ConfiguredLimits == nil) ||
		(result.ConfiguredLimits != nil && expected.ConfiguredLimits != nil && *result.ConfiguredLimits != *expected.ConfiguredLimits) {
		t.Errorf("configuredLimits mismatch for %s: got %v want %v", name, result.ConfiguredLimits, expected.ConfiguredLimits)
	}
	// Usage comparison.
	if len(expected.AttributedUsage) > 0 {
		compareUsage(t, name, "attributedUsage", result.AttributedUsage, expected.AttributedUsage)
	}
	if len(expected.TerminalUsage) > 0 {
		compareUsage(t, name, "terminalUsage", result.TerminalUsage, expected.TerminalUsage)
	}
	if len(expected.UnattributedUsage) > 0 {
		compareUsage(t, name, "unattributedUsage", result.UnattributedUsage, expected.UnattributedUsage)
	}
	if len(expected.UnframedAttributedUsage) > 0 {
		compareUsage(t, name, "unframedAttributedUsage", result.UnframedAttributedUsage, expected.UnframedAttributedUsage)
	}
	// UsageComplete.
	if expected.UsageComplete && !result.UsageComplete {
		t.Errorf("usageComplete mismatch for %s: got false want true", name)
	}
	// Attempts.
	compareJSONArrays(t, name, "attempts", result.Attempts, expected.Attempts)
	// Retries.
	compareJSONArrays(t, name, "retries", result.Retries, expected.Retries)
	// ValidationLinks.
	compareJSONArrays(t, name, "validationLinks", result.ValidationLinks, expected.ValidationLinks)
	// Frames.
	compareJSONArrays(t, name, "frames", projectFixtureFrames(result.Frames), expected.Frames)
	// Payloads.
	compareJSONArrays(t, name, "payloads", result.Payloads, expected.Payloads)
	// Gaps.
	compareJSONArrays(t, name, "gaps", result.Gaps, expected.Gaps)
	// Uncertainties: not written as a separate index; compare counts via manifest.
	// The manifest's UncertaintyCount is checked separately.
}

// fixtureFrameResult is the Java-produced semantic frame shape. Go's internal
// frame index also carries query-only cross references and completeness flags;
// those are covered by focused Go tests rather than added to the Java corpus.
type fixtureFrameResult struct {
	FrameID                 string  `json:"frameId"`
	ParentFrameID           *string `json:"parentFrameId"`
	FrameType               string  `json:"frameType"`
	Route                   string  `json:"route,omitempty"`
	InclusiveDurationMillis *int64  `json:"inclusiveDurationMillis"`
	SelfDurationMillis      *int64  `json:"selfDurationMillis"`
	DirectUsage             Usage   `json:"directUsage"`
	DescendantUsage         Usage   `json:"descendantUsage"`
	InclusiveUsage          Usage   `json:"inclusiveUsage"`
}

func projectFixtureFrames(frames []frameResult) []fixtureFrameResult {
	out := make([]fixtureFrameResult, len(frames))
	for i, frame := range frames {
		out[i] = fixtureFrameResult{
			FrameID: frame.FrameID, ParentFrameID: frame.ParentFrameID, FrameType: frame.FrameType,
			Route: frame.Route, InclusiveDurationMillis: frame.InclusiveDurationMillis,
			SelfDurationMillis: frame.SelfDurationMillis, DirectUsage: frame.DirectUsage,
			DescendantUsage: frame.DescendantUsage, InclusiveUsage: frame.InclusiveUsage,
		}
	}
	return out
}

// compareUsage compares a Usage value against a JSON RawMessage from the
// expected file.
func compareUsage(t *testing.T, name, field string, got Usage, expectedRaw json.RawMessage) {
	t.Helper()
	var expected Usage
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatalf("parse %s for %s: %v", field, name, err)
	}
	if got != expected {
		t.Errorf("%s mismatch for %s: got %+v want %+v", field, name, got, expected)
	}
}

// compareJSONArrays compares a calculated slice against the expected JSON array
// by normalizing both to canonical JSON. Nil slices are treated as empty arrays
// to match the Java fixture corpus convention.
func compareJSONArrays[T any](t *testing.T, name, field string, got []T, expectedRaw json.RawMessage) {
	t.Helper()
	// Treat nil and empty as equivalent.
	if len(got) == 0 {
		got = []T{}
	}
	// Normalize expected: if it's null or missing, treat as empty array.
	var expNorm any
	if len(expectedRaw) == 0 || string(expectedRaw) == "null" {
		expNorm = []any{}
	} else if err := json.Unmarshal(expectedRaw, &expNorm); err != nil {
		t.Fatalf("normalize expected %s for %s: %v", field, name, err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal %s for %s: %v", field, name, err)
	}
	var gotNorm any
	if err := json.Unmarshal(gotJSON, &gotNorm); err != nil {
		t.Fatalf("normalize got %s for %s: %v", field, name, err)
	}
	gotCanonical, _ := json.Marshal(gotNorm)
	expCanonical, _ := json.Marshal(expNorm)
	if string(gotCanonical) != string(expCanonical) {
		t.Errorf("%s mismatch for %s:\n got: %s\nwant: %s", field, name, gotCanonical, expCanonical)
	}
}

// categoryOfSafe returns the category string or "<none>".
func categoryOfSafe(err *consolecore.Error) string {
	cat, ok := categoryOf(err)
	if !ok {
		return "<none>"
	}
	return string(cat)
}

// bytesReader wraps a byte slice as an io.Reader.
func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
