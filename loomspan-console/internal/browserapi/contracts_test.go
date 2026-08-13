package browserapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

type fixtureBootstrap struct {
	ProcessID          string            `json:"processId"`
	ConsoleVersion     string            `json:"consoleVersion"`
	WorkspacePath      string            `json:"workspacePath"`
	TabID              string            `json:"tabId"`
	CSRFToken          string            `json:"csrfToken"`
	TargetFormDefaults map[string]string `json:"targetFormDefaults"`
	Target             targetDTO         `json:"target"`
}

func TestBrowserTargetFixtureCorpusMatchesCommittedInventoryByteForByte(t *testing.T) {
	observed := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	scope := "11111111-1111-4111-8111-111111111111"
	base := fixtureBootstrap{ProcessID: "process-1", ConsoleVersion: "0.1.0-SNAPSHOT", WorkspacePath: `C:\workspace`, TabID: "tab-1", CSRFToken: "csrf-1", TargetFormDefaults: map[string]string{"address": "", "applicationKey": ""}}
	noTarget := base
	noTarget.Target = targetDTO{Status: consolecore.NoTargetStatus(observed)}
	required := base
	required.Target = targetDTO{
		Address: "https://application.example/context",
		Status: consolecore.StatusSnapshot{
			ObservedAt: observed, TargetScopeID: scope, TargetSelection: consolecore.SelectionSelected,
			TargetConnection: consolecore.ConnectionUnknown, TargetAuthentication: consolecore.AuthenticationRequired,
			JavaGoCompatibility: consolecore.CompatibilityNotChecked,
			RuntimeIdentity:     consolecore.RuntimeNotEstablished, LiveMonitoring: consolecore.LiveUnknown,
		},
	}
	connected := required
	connected.Target.Status.TargetConnection = consolecore.ConnectionReachable
	connected.Target.Status.TargetAuthentication = consolecore.AuthenticationEstablished
	connected.Target.Status.JavaGoCompatibility = consolecore.CompatibilityCompatible
	connected.Target.Status.RuntimeIdentity = consolecore.RuntimeEstablished
	connected.Target.Status.InstanceID = "22222222-2222-4222-8222-222222222222"
	connected.Target.Status.LiveMonitoring = consolecore.LiveAvailable

	expected := map[string]any{
		"bootstrap-no-target.json":               noTarget,
		"bootstrap-authentication-required.json": required,
		"bootstrap-connected.json":               connected,
		"error-authentication-required.json": errorEnvelope{Error: browserError{
			Code: "TARGET_AUTHENTICATION_REQUIRED", Message: "The application key was rejected.", TargetScopeID: scope,
		}},
		"error-access-blocked.json": errorEnvelope{Error: browserError{
			Code: "TARGET_ACCESS_BLOCKED", Message: "The selected target denied access before Loomspan authentication.", TargetScopeID: scope,
		}},
		"error-unavailable.json": errorEnvelope{Error: browserError{
			Code: "TARGET_UNAVAILABLE", Message: "The selected target is unavailable.", TargetScopeID: scope,
			Details: consolecore.Details{TransportCategory: "timeout"},
		}},
		"error-incompatible.json": errorEnvelope{Error: browserError{
			Code: "INCOMPATIBLE_TARGET", Message: "The selected target uses a different Loomspan release.", TargetScopeID: scope,
			Details: consolecore.Details{ExpectedCompatibilityVersion: "0.1.0-SNAPSHOT", ObservedCompatibilityVersion: "0.1.0"},
		}},
		"error-target-changed.json": errorEnvelope{Error: browserError{
			Code: "TARGET_CHANGED", Message: "The selected target changed. Start this operation again.", TargetScopeID: scope,
			Details: consolecore.Details{CurrentTargetScopeID: "33333333-3333-4333-8333-333333333333"},
		}},
	}
	root := filepath.Join("..", "..", "browser-fixtures", "target")
	assertFixtureCorpus(t, root, expected)
}

// TestBrowserArtifactFixtureCorpusMatchesCommittedInventoryByteForByte proves
// that the committed artifact fixture JSON files in
// browser-fixtures/artifacts/ match the Go DTOs byte-for-byte. This covers the
// Phase 3 acquire-response and storage-snapshot DTOs, ensuring the wire
// contract is stable and the TypeScript contracts.ts types match the real
// backend response shape.
func TestBrowserArtifactFixtureCorpusMatchesCommittedInventoryByteForByte(t *testing.T) {
	finalizedAt := time.Date(2026, 7, 27, 10, 10, 0, 0, time.UTC)
	acquiredAt := time.Date(2026, 7, 27, 10, 15, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 7, 27, 10, 20, 0, 0, time.UTC)
	appExpiresAt := time.Date(2026, 7, 27, 11, 10, 0, 0, time.UTC)

	expected := map[string]any{
		"acquire-response.json": acquiredArtifactDTO{
			Source:        evidence.SourceTarget,
			Handle:        "01-handletoken",
			TraceID:       "trace-1",
			SessionID:     "session-1",
			Outcome:       "SUCCEEDED",
			FinalizedAt:   finalizedAt,
			LocalBytes:    4096,
			AcquiredAt:    acquiredAt,
			LastUsedAt:    acquiredAt,
			ExpiresAt:     expiresAt,
			HasIdleExpiry: true,
		},
		"storage-snapshot.json": storageSnapshotDTO{
			WorkspaceLabel: "work",
			MaxBytes:       1048576,
			Unlimited:      false,
			IdleTTL:        "5m0s",
			NeverExpire:    false,
			ChargedBytes:   4096,
			AcquiredCount:  1,
			Entries: []artifact.StoredEntry{
				{
					Source:                    evidence.SourceTarget,
					TargetScopeID:             "11111111-1111-4111-8111-111111111111",
					TraceID:                   "trace-1",
					SessionID:                 "session-1",
					Outcome:                   "SUCCEEDED",
					PersistencePolicy:         "RETAINED",
					FinalizedAt:               finalizedAt,
					AcquiredAt:                acquiredAt,
					LastUsedAt:                acquiredAt,
					ExpiresAt:                 expiresAt,
					HasIdleExpiry:             true,
					LocalBytes:                4096,
					ApplicationTraceExpiresAt: &appExpiresAt,
					ApplicationAvailability:   "AVAILABLE",
					LocalAvailable:            true,
					ActivePin:                 false,
				},
			},
		},
	}
	root := filepath.Join("..", "..", "browser-fixtures", "artifacts")
	assertFixtureCorpus(t, root, expected)
}

func TestBrowserTraceAnalysisFixtureCorpusMatchesCommittedInventoryByteForByte(t *testing.T) {
	scope := "11111111-1111-4111-8111-111111111111"
	duration := int64(12)
	opened := int64(100)
	closed := int64(112)
	next := "opaque-continuation"
	expected := map[string]any{
		"summary.json":                   summaryDTO{Source: evidence.SourceTarget, TargetScopeID: scope, TraceID: "trace-1", SessionID: "session-1", Outcome: "FAILED", ConfiguredLimits: &configuredLimitsDTO{MaxSkillInvocations: 7, MaxToolInvocations: 11, MaxLinterRetries: 3, MaxModelCalls: 5, MaxProviderAttempts: 15, MaxUsageUnits: 1234}, RecordCount: 4, FrameCount: 1, RootFrameIDs: []string{}, UsageComplete: false},
		"frames.json":                    pageDTO[frameDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []frameDTO{{FrameID: "frame-1", ChildFrameIDs: []string{}, FrameType: "SKILL", Route: "hello", OpenedTimestampMillis: opened, ClosedTimestampMillis: &closed, InclusiveDurationMillis: &duration, DirectUsage: usageValueDTO{PromptUnits: 10, CompletionUnits: 2, TotalUnits: 12}, DirectUsageComplete: true, InclusiveUsage: usageValueDTO{PromptUnits: 10, CompletionUnits: 2, TotalUnits: 12}, InclusiveUsageComplete: true, SkillNames: []string{"registered.skill"}, Outcomes: []string{"FAILED"}, AttemptIDs: []string{"attempt-1"}, RetrySequenceIDs: []string{"retry-1"}, ValidationStatuses: []string{"exhausted"}, FailureIDs: []string{"failure-1"}}}, HasMore: true, NextCursor: &next},
		"records.json":                   pageDTO[recordDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []recordDTO{{Sequence: 3, Type: "MODEL_RESPONSE_RECEIVED", FrameID: "frame-1", FrameType: "SKILL", Route: "hello", ThreadName: "worker-1", TimestampMillis: 112, Representation: "logical", IsEnvelope: true, PayloadID: "payload-1"}}, HasMore: false},
		"usage.json":                     usageDTO{Source: evidence.SourceTarget, TargetScopeID: scope, Attributed: usageValueDTO{PromptUnits: 10, CompletionUnits: 2, TotalUnits: 12}},
		"attempts.json":                  pageDTO[attemptDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []attemptDTO{}, HasMore: false},
		"retries.json":                   pageDTO[retryDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []retryDTO{{RetrySequenceID: "retry-1", Usage: usageValueDTO{TotalUnits: 12}, UsageComplete: false}}, HasMore: true, NextCursor: &next},
		"validation-links.json":          pageDTO[validationDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []validationDTO{{Status: "VALID", RetrySequenceID: "retry-1", AttemptID: "attempt-1", AttemptNumber: 2}}, HasMore: false},
		"failures.json":                  pageDTO[failureDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []failureDTO{{FailureID: "failure-1", Terminal: true, Sequence: 42, TimestampMillis: 1000, RecordType: "ERROR_RECORDED", FrameID: "frame-1", Route: "hello", AttemptID: "attempt-1", RetrySequenceID: "retry-1", ValidationStatus: "exhausted"}}, HasMore: false},
		"payloads.json":                  pageDTO[payloadDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []payloadDTO{}, HasMore: false},
		"gaps.json":                      pageDTO[gapDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []gapDTO{{Kind: "MISSING_TIMESTAMP", FrameID: "frame-1"}}, HasMore: false},
		"uncertainties.json":             pageDTO[uncertaintyDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []uncertaintyDTO{{Kind: "INCOMPLETE_DURATION", FrameID: "frame-1"}}, HasMore: false},
		"search.json":                    pageDTO[searchDTO]{Source: evidence.SourceTarget, TargetScopeID: scope, Items: []searchDTO{{Sequence: 3, RecordType: "MODEL_RESPONSE_RECEIVED", FrameID: "frame-1", MatchOffset: 2, MatchLength: 4, SearchedField: "payload"}}, HasMore: true, NextCursor: &next},
		"range.json":                     map[string]any{"source": evidence.SourceTarget, "targetScopeId": scope, "actualStart": int64(0), "actualEnd": int64(4), "totalLength": int64(4), "contentType": "text/plain", "encoding": "TEXT", "content": "<a>", "hasMore": false, "nextCursor": nil},
		"base64-range-continuation.json": map[string]any{"source": evidence.SourceTarget, "targetScopeId": scope, "actualStart": int64(4), "actualEnd": int64(8), "totalLength": int64(12), "contentType": "application/octet-stream", "encoding": "BASE64", "content": "AQIDBA==", "hasMore": true, "nextCursor": next},
	}
	assertFixtureCorpus(t, filepath.Join("..", "..", "browser-fixtures", "trace-analysis"), expected)
}

// assertFixtureCorpus verifies that every file in root matches a Go value in
// expected, marshaled to JSON with a trailing newline. It checks that the
// directory contains exactly the expected files (no more, no fewer) and that
// each file's bytes match the marshaled DTO exactly.
func assertFixtureCorpus(t *testing.T, root string, expected map[string]any) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	expectedNames := make([]string, 0, len(expected))
	for name := range expected {
		expectedNames = append(expectedNames, name)
	}
	sort.Strings(expectedNames)
	if !equalStrings(names, expectedNames) {
		t.Fatalf("fixture inventory=%v expected=%v", names, expectedNames)
	}
	for name, value := range expected {
		generated, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		generated = append(generated, '\n')
		committed, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(generated, committed) {
			t.Errorf("%s differs\nwant %s\ngot  %s", name, generated, committed)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
