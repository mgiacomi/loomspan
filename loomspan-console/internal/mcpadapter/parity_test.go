package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
)

// WF-X-R7/WF-SP-R7/WF-SP-R9: browser responses serialize the shared
// observability DTO directly; MCP preserves skill identity, source path, YAML,
// and observation facts while omitting internal ownership identifiers.
func TestBrowserAndMCPPreserveSameSkillFacts(t *testing.T) {
	browserFact := observability.SkillDetail{
		RegisteredName: "skill-☃", SourcePath: `C:\\deployed\\skill.yaml`,
		Yaml: "name: skill-☃\ndescription: data only\n",
	}
	body, err := json.Marshal(browserFact)
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return body, nil })
	_, envelope, err := handleGetSkill(context.Background(), options, getSkillInput{RegisteredName: browserFact.RegisteredName})
	if err != nil || envelope.Result == nil {
		t.Fatalf("envelope=%#v err=%v", envelope, err)
	}
	mcpFact := envelope.Result.Skill
	if mcpFact.RegisteredName != browserFact.RegisteredName || mcpFact.SourcePath != browserFact.SourcePath || mcpFact.YAML != browserFact.Yaml {
		t.Fatalf("browser=%#v MCP=%#v", browserFact, mcpFact)
	}
	if envelope.Result.ObservedAt != options.Now().UTC() {
		t.Fatalf("MCP observation fact changed: %#v", envelope.Result)
	}
}

// WF-SE-R2/WF-SE-R6/WF-SE-R9/WF-SE-R10: compare every bounded active
// execution fact, while deliberately excluding completed-history claims.
func TestBrowserAndMCPPreserveSameActiveExecutionFacts(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-execution-detail.json"))
	if err != nil {
		t.Fatal(err)
	}
	var browserFact observability.ActiveExecution
	if err := json.Unmarshal(fixture, &browserFact); err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return fixture, nil })
	_, envelope, err := handleGetExecution(context.Background(), options, getExecutionInput{SessionID: browserFact.SessionID})
	if err != nil || envelope.Result == nil {
		t.Fatalf("envelope=%#v err=%v", envelope, err)
	}
	want := mapExecution(browserFact)
	if !reflect.DeepEqual(envelope.Result.Execution, want) {
		t.Fatalf("browser=%#v MCP=%#v", browserFact, envelope.Result.Execution)
	}
}

// WF-X-R6/WF-X-R7/WF-FE-R10/WF-SE-R3/WF-SE-R9: MCP changes only the
// wrapper. Ordered activity, query time, continuity time, reset, and gap facts
// retain the meanings used by the browser recent-activity response.
func TestBrowserAndMCPPreserveSameRecentContinuityAndGapFacts(t *testing.T) {
	sequence := int64(9)
	browserItem := live.Activity{
		InstanceID: mcpTestInstanceID, Cursor: "9", SessionID: "session-1", TraceID: "trace-1",
		CanonicalSequence: &sequence, Timestamp: time.Date(2026, 8, 13, 20, 0, 9, 0, time.UTC),
		Kind: live.KindModelAttemptFailed, Summary: "failed", Details: json.RawMessage(`{"attempt":2}`),
	}
	mcpItem, err := mapActivity(browserItem)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(mcpItem)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip live.Activity
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.InstanceID != "" || roundTrip.Cursor != browserItem.Cursor || roundTrip.SessionID != browserItem.SessionID ||
		roundTrip.TraceID != browserItem.TraceID || !reflect.DeepEqual(roundTrip.CanonicalSequence, browserItem.CanonicalSequence) ||
		!roundTrip.Timestamp.Equal(browserItem.Timestamp) || roundTrip.Kind != browserItem.Kind || roundTrip.Summary != browserItem.Summary ||
		string(roundTrip.Details) != string(browserItem.Details) {
		t.Fatalf("browser=%#v MCP round trip=%#v", browserItem, roundTrip)
	}
	queryObservedAt := time.Date(2026, 8, 13, 21, 0, 0, 0, time.UTC)
	continuityObservedAt := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	continuity := &live.Continuity{
		IntervalID: "interval-2", TargetScopeID: "scope-1", InstanceID: mcpTestInstanceID,
		FirstCursor: "9", LastCursor: "9", ObservedAt: continuityObservedAt,
		Reset: &live.ResetFact{Cause: live.ResetUpstreamStaleCursor, Cursor: "8", Timestamp: continuityObservedAt.Add(-time.Second)},
	}
	mcpContinuity := mapContinuity(continuity)
	mcpResult := activityResult{
		ObservedAt: queryObservedAt,
		Items:      []activityDTO{mcpItem}, ReturnedCursorRange: &cursorRangeDTO{FirstCursor: "9", LastCursor: "9"},
		Continuity: mcpContinuity, Coverage: coverageDTO{GlobalEvictedThroughCursor: "7", SessionStartCursor: "9", SessionRetainedCursorRange: &cursorRangeDTO{FirstCursor: "9", LastCursor: "9"}},
	}
	if mcpResult.ObservedAt != queryObservedAt || mcpResult.Continuity != mcpContinuity || mcpResult.Coverage.SessionStartCursor != "9" {
		t.Fatalf("recent observation/continuity/gap facts changed: %#v", mcpResult)
	}
}

// WF-X-R10: browser and MCP wrappers differ, but the shared domain meaning is
// preserved and the internal cause is never promoted into the MCP DTO.
func TestBrowserAndMCPPreserveSameDomainErrorMeanings(t *testing.T) {
	browserFact := consolecore.NewError(
		consolecore.CodeNotFound, "The requested skill was not found.", "scope-1",
		consolecore.Details{CurrentTargetScopeID: "scope-1"},
		context.Canceled,
	)
	mcpFact := mapDomainError(browserFact)
	if mcpFact.Code != browserFact.Code || mcpFact.Message != browserFact.Message || !reflect.DeepEqual(mcpFact.Details, errorDetailsDTO{}) {
		t.Fatalf("browser=%#v MCP=%#v", browserFact, mcpFact)
	}
	encoded, err := json.Marshal(mcpFact)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || bytes.Contains(encoded, []byte(context.Canceled.Error())) {
		t.Fatalf("domain error leaked its internal cause: %s", encoded)
	}
}
