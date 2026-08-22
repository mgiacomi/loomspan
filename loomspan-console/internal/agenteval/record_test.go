package agenteval

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluationRecordRoundTripAndSanitization(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	record := completeRecord(cases["failed-execution"])
	if err := ValidateRecord(record, cases); err != nil {
		t.Fatal(err)
	}
	first, _ := CanonicalJSON(record)
	second, _ := CanonicalJSON(record)
	if string(first) != string(second) {
		t.Fatal("canonical record JSON is not deterministic")
	}
}

func TestEvaluationRecordRejectsSecretsHeadersPathsAndRawSensitiveContent(t *testing.T) {
	cases, _ := LoadCases()
	for _, unsafe := range []string{
		"lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"Authorization: Bearer redacted",
		"C:\\Users\\developer\\secret.txt",
		"/home/developer/secret.txt",
	} {
		t.Run(strings.ReplaceAll(unsafe[:4], "\\", "-"), func(t *testing.T) {
			record := completeRecord(cases["failed-execution"])
			record.FinalAnswer += unsafe
			if err := ValidateRecord(record, cases); err == nil {
				t.Fatalf("unsafe record value was accepted: %q", unsafe)
			}
		})
	}
}

func TestHeadlessEvaluationRequiresCompleteClientEventStreamForToolSafetyGate(t *testing.T) {
	cases, _ := LoadCases()
	record := completeRecord(cases["failed-execution"])
	record.EventStreamComplete = false
	if err := ValidateRecord(record, cases); err == nil {
		t.Fatal("incomplete headless event stream was accepted")
	}
	record.EventStreamKind = "gui-manual"
	if err := ValidateRecord(record, cases); err != nil {
		t.Fatalf("manual GUI observation should remain explicitly non-headless: %v", err)
	}
}

func TestEvaluationRecordRequiresWorkflowProtocolAndStableIdentifierClasses(t *testing.T) {
	cases, _ := LoadCases()
	caseValue := cases["failed-execution"]
	for name, mutate := range map[string]func(*EvaluationRecord){
		"workflow":    func(record *EvaluationRecord) { record.WorkflowIDs = []string{"invented"} },
		"protocol":    func(record *EvaluationRecord) { record.MCPProtocol = "" },
		"identifiers": func(record *EvaluationRecord) { delete(record.Identifiers, "traceId") },
	} {
		t.Run(name, func(t *testing.T) {
			record := completeRecord(caseValue)
			mutate(&record)
			if err := ValidateRecord(record, cases); err == nil {
				t.Fatalf("record with invalid %s was accepted", name)
			}
		})
	}
}

func completeRecord(caseValue Case) EvaluationRecord {
	rubric := make(map[string]RubricResult)
	for _, dimension := range RubricDimensions {
		rubric[dimension] = RubricResult{Score: 4, Note: "reviewed against fixture evidence"}
	}
	operations := []Operation{}
	if (caseValue.MCPAvailable == nil || *caseValue.MCPAvailable) && caseValue.ID != "unsupported-protocol" {
		operations = append(operations, Operation{Tool: "LOOMSPAN_get_runtime", ArgumentHash: strings.Repeat("a", 64), ResultHash: strings.Repeat("b", 64)})
	}
	identifiers := make(map[string][]string)
	for _, identifierClass := range caseValue.RequiredIdentifierClasses {
		identifiers[identifierClass] = []string{identifierClass + "-fixture"}
	}
	return EvaluationRecord{
		SchemaVersion: RecordSchemaVersion, RunID: "run-1", ConversationID: "conversation-1", CaseID: caseValue.ID,
		WorkflowIDs: caseValue.WorkflowIDs, RecordedAt: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC), OS: "windows/amd64",
		ClientProduct: "Codex CLI", ClientBuild: "example-build", Model: "example-model",
		ConsoleVersion: "0.1.0-SNAPSHOT", ConsoleCommit: "0123456789abcdef", RunOrdinal: 1, MCPProtocol: "2026-07-28",
		Capabilities: caseValue.Capabilities, Operations: operations, Identifiers: identifiers, FinalAnswer: "fixture-grounded answer",
		SupportedFacts: caseValue.ExpectedFacts, Limitations: caseValue.LimitationFacts,
		EventStreamKind: "headless", EventStreamComplete: true, ClientEvents: []ClientEvent{}, Rubric: rubric,
		HardGateFailures: []string{},
	}
}
