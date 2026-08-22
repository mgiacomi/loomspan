package mcpadapter

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

func validateCompactInstance(t *testing.T, schema *jsonschema.Schema, instance any) error {
	t.Helper()
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved.Validate(instance)
}

func validateTypedOutput[T any](t *testing.T, name string, schema *jsonschema.Schema, output T) {
	t.Helper()
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(encoded, &instance); err != nil {
		t.Fatal(err)
	}
	if err := validateCompactInstance(t, schema, instance); err != nil {
		t.Fatalf("compact contract rejected %s output: %v\n%s", name, err, encoded)
	}
	if err := newCompleteOutputValidator[T](name)(output); err != nil {
		t.Fatal(err)
	}
}

func validateTypedEnvelope[T any](t *testing.T, name string, schema *jsonschema.Schema, result T) {
	t.Helper()
	validateTypedOutput(t, name+" success", schema, toolEnvelope[T]{Result: &result})
	domain := domainErrorDTO{Code: consolecore.CodeInvalidArgument, Message: "invalid", Details: errorDetailsDTO{}}
	validateTypedOutput(t, name+" domain error", schema, toolEnvelope[T]{Error: &domain})
}

func TestCompactEnvelopeRequiresExactlyOneResultOrError(t *testing.T) {
	schema := compactEnvelopeSchema(compactObject([]string{"id"}, map[string]*jsonschema.Schema{"id": compactString()}, true))
	valid := []any{
		map[string]any{"result": map[string]any{"id": "one"}},
		map[string]any{"error": map[string]any{"code": "NOT_FOUND", "message": "missing", "details": map[string]any{"extra": true}}},
	}
	for _, instance := range valid {
		if err := validateCompactInstance(t, schema, instance); err != nil {
			t.Fatalf("valid instance %#v: %v", instance, err)
		}
	}
	invalid := []any{
		map[string]any{},
		map[string]any{"result": map[string]any{"id": "one"}, "error": map[string]any{"code": "X", "message": "x"}},
		map[string]any{"result": map[string]any{"id": "one"}, "extra": true},
		map[string]any{"error": map[string]any{"code": "X"}},
	}
	for _, instance := range invalid {
		if err := validateCompactInstance(t, schema, instance); err == nil {
			t.Fatalf("invalid instance accepted: %#v", instance)
		}
	}
}

type malformedCompleteOutput struct {
	Required string `json:"required"`
}

func (malformedCompleteOutput) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }

func TestCompleteOutputValidationRejectsFieldsHiddenByCompactDiscovery(t *testing.T) {
	compact := compactOpenObject()
	var compactInstance any
	if err := json.Unmarshal([]byte(`{}`), &compactInstance); err != nil {
		t.Fatal(err)
	}
	if err := validateCompactInstance(t, compact, compactInstance); err != nil {
		t.Fatalf("compact control rejected: %v", err)
	}
	validate := newCompleteOutputValidator[malformedCompleteOutput]("test-tool")
	err := validate(malformedCompleteOutput{Required: "present before custom serialization"})
	if err == nil || !strings.Contains(err.Error(), "complete contract") {
		t.Fatalf("complete validation error = %v", err)
	}
}

func TestEveryInstalledToolRepresentativeOutputsValidateAgainstBothSchemas(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	evidence := evidenceDTO{TraceID: "trace", SessionID: "session", ObservedAt: now}
	tests := []struct {
		name     string
		validate func(*testing.T)
	}{
		{"runtime", func(t *testing.T) {
			validateTypedOutput(t, "runtime success", runtimeOutputSchema(), RuntimeOutput{Capabilities: []string{}, Status: runtimeStatusDTO{ObservedAt: now, TargetSelection: consolecore.SelectionNone, TargetConnection: consolecore.ConnectionNotApplicable, TargetAuthentication: consolecore.AuthenticationNotApplicable, JavaGoCompatibility: consolecore.CompatibilityNotApplicable, RuntimeIdentity: consolecore.RuntimeNotApplicable, LiveMonitoring: consolecore.LiveNotApplicable}})
		}},
		{"skills-list", func(t *testing.T) {
			validateTypedEnvelope(t, "skills-list", skillListOutputSchema(), skillListResult{ObservedAt: now, Items: []skillSummaryDTO{}})
		}},
		{"skill-detail", func(t *testing.T) {
			validateTypedEnvelope(t, "skill-detail", skillDetailOutputSchema(), skillDetailResult{ObservedAt: now, Skill: skillDetailDTO{RegisteredName: "skill", SourcePath: "skill.yaml", YAML: "name: skill"}})
		}},
		{"executions-list", func(t *testing.T) {
			validateTypedEnvelope(t, "executions-list", executionListOutputSchema(), executionListResult{ObservedAt: now, Items: []executionDTO{}})
		}},
		{"execution-detail", func(t *testing.T) {
			validateTypedEnvelope(t, "execution-detail", executionDetailOutputSchema(), executionDetailResult{ObservedAt: now, Execution: executionDTO{SessionID: "session", TraceID: "trace", Status: "RUNNING", Phase: "STEP", ActivePath: []framePathDTO{}}})
		}},
		{"activity", func(t *testing.T) {
			validateTypedEnvelope(t, "activity", activityOutputSchema(), activityResult{ObservedAt: now, Items: []activityDTO{}})
		}},
		{"traces-list", func(t *testing.T) {
			validateTypedEnvelope(t, "traces-list", traceListOutputSchema(), listTracesResult{ObservedAt: now, Items: []traceInventoryItemDTO{}, Complete: true, Limitations: []traceLimitationDTO{}})
		}},
		{"trace-summary", func(t *testing.T) {
			terminalFailureID := "failure-terminal"
			validateTypedEnvelope(t, "trace-summary", traceSummaryOutputSchema(), getTraceResult{Evidence: evidence, Summary: traceSummaryDTO{Outcome: "FAILED", TerminalFailureID: &terminalFailureID, RecordCountsByType: map[string]int64{}, RootFrameIDs: []string{"root"}, UsageComplete: true}})
		}},
		{"frames", func(t *testing.T) {
			validateTypedEnvelope(t, "frames", frameQueryOutputSchema(), queryFramesResult{Evidence: evidence, Projection: "COMPACT", Items: []frameDTO{}})
		}},
		{"records", func(t *testing.T) {
			validateTypedEnvelope(t, "records", recordQueryOutputSchema(), queryRecordsResult{Evidence: evidence, Items: []recordDTO{{Sequence: 1, Type: "MODEL_RESPONSE_RECEIVED", ThreadName: "main", TimestampMillis: 1, Representation: "LOGICAL", Content: &contentDescriptorDTO{Role: "DATA", Available: true, Complete: true, ContentRef: "opaque"}}}})
			descriptors := []searchContentDescriptorDTO{{ContentID: "c1", ContentRef: "opaque"}}
			search := queryRecordsResult{Evidence: evidence, Matches: []searchMatchDTO{{Sequence: 1, RecordType: "MODEL_RESPONSE_RECEIVED", MatchOffset: 2, MatchLength: 4, SearchedField: "content", ContentID: "c1"}}, ContentDescriptors: &descriptors, Search: &searchCoverageDTO{Query: "needle", CaseSensitive: true, Representation: "LOGICAL", SearchedFields: []string{"metadata", "content"}, SemanticContentCoverage: "AVAILABLE_COMPLETE_TEXT", WorkComplete: true, Limitations: []traceLimitationDTO{}}}
			validateTypedOutput(t, "records search success", recordQueryOutputSchema(), toolEnvelope[queryRecordsResult]{Result: &search})
		}},
		{"content-range", func(t *testing.T) {
			validateTypedEnvelope(t, "content-range", rangeOutputSchema(), rangeResult{Evidence: evidence, ContentType: "text/plain", Encoding: "TEXT", Content: "value"})
		}},
		{"artifact-range", func(t *testing.T) {
			validateTypedEnvelope(t, "artifact-range", rangeOutputSchema(), rangeResult{Evidence: evidence, ContentType: "application/x-ndjson", Encoding: "TEXT", Content: "{}"})
		}},
	}
	if len(tests) != 12 {
		t.Fatalf("tool fixtures=%d", len(tests))
	}
	for _, test := range tests {
		t.Run(test.name, test.validate)
	}
}

func TestCompactSchemasRetainDecisionAndNavigationFields(t *testing.T) {
	summary := traceSummaryOutputSchema().Properties["result"].Properties["summary"]
	terminalFailure := summary.Properties["terminalFailureId"]
	if terminalFailure == nil || terminalFailure.Type != "string" {
		t.Fatalf("terminalFailureId schema=%+v", terminalFailure)
	}

	tests := []struct {
		name     string
		schema   *jsonschema.Schema
		instance any
	}{
		{"runtime", runtimeOutputSchema(), map[string]any{"capabilities": []any{}, "status": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "targetSelection": "NONE", "targetConnection": "DISCONNECTED", "targetAuthentication": "UNKNOWN", "javaGoCompatibility": "UNKNOWN", "runtimeIdentity": "UNKNOWN", "liveMonitoring": "UNAVAILABLE"}}},
		{"skills-list", skillListOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "items": []any{map[string]any{"registeredName": "skill", "sourcePath": "skill.yaml"}}, "hasMore": false}}},
		{"skill-detail", skillDetailOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "skill": map[string]any{"registeredName": "skill", "sourcePath": "skill.yaml", "yaml": "name: skill"}}}},
		{"executions-list", executionListOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "items": []any{map[string]any{"sessionId": "s", "traceId": "t", "entrySkill": "skill", "status": "RUNNING", "phase": "STEP"}}, "hasMore": false}}},
		{"execution-detail", executionDetailOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "execution": map[string]any{"sessionId": "s", "traceId": "t", "entrySkill": "skill", "status": "RUNNING", "phase": "STEP", "activePath": []any{}, "usage": map[string]any{}, "configuredLimits": map[string]any{}}}}},
		{"activity", activityOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "items": []any{map[string]any{"cursor": "c", "sessionId": "s", "traceId": "t", "timestamp": "2026-08-19T00:00:00Z", "kind": "STEP_COMPLETED", "summary": "done", "details": map[string]any{}}}, "hasMore": false, "coverage": map[string]any{}}}},
		{"traces-list", traceListOutputSchema(), map[string]any{"result": map[string]any{"observedAt": "2026-08-19T00:00:00Z", "items": []any{map[string]any{"traceId": "t", "evidenceSources": []any{"TARGET"}}}, "complete": true, "hasMore": false}}},
		{"trace-summary", traceSummaryOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "summary": map[string]any{"outcome": "FAILED", "terminalFailureId": "failure-terminal", "recordCount": 1, "recordCountsByType": map[string]any{"TRACE_COMPLETED": 1}, "frameCount": 1, "attemptCount": 1, "retryCount": 0, "failureCount": 1, "rootFrameIds": []any{"root"}, "usageComplete": true}}}},
		{"frames", frameQueryOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "projection": "COMPACT", "items": []any{map[string]any{"frameId": "f", "childFrameIds": []any{}, "frameType": "ROOT_MISSION", "openedTimestampMillis": 1, "inclusiveDurationMillis": 42, "outcome": "failed"}}, "hasMore": false}}},
		{"records", recordQueryOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "items": []any{map[string]any{"sequence": 1, "type": "STEP_COMPLETED", "threadName": "main", "timestampMillis": 1, "representation": "LOGICAL", "raw": map[string]any{}, "facts": map[string]any{}}}, "hasMore": false}}},
		{"record-content", recordQueryOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "items": []any{map[string]any{"sequence": 1, "type": "MODEL_RESPONSE_RECEIVED", "threadName": "main", "timestampMillis": 1, "representation": "LOGICAL", "raw": map[string]any{}, "facts": map[string]any{}, "content": map[string]any{"role": "DATA", "available": true, "complete": true, "contentRef": "opaque"}}}, "hasMore": false}}},
		{"record-search-descriptor", recordQueryOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "matches": []any{map[string]any{"sequence": 1, "recordType": "MODEL_RESPONSE_RECEIVED", "matchOffset": 2, "matchLength": 4, "searchedField": "content", "contentId": "c1"}}, "contentDescriptors": []any{map[string]any{"contentId": "c1", "contentRef": "opaque"}}, "search": map[string]any{}, "hasMore": false}}},
		{"content-range", rangeOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "actualStart": 0, "actualEnd": 1, "totalLength": 1, "contentType": "text/plain", "encoding": "UTF-8", "content": "x", "hasMore": false}}},
		{"artifact-range", rangeOutputSchema(), map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "actualStart": 0, "actualEnd": 1, "totalLength": 1, "contentType": "application/x-ndjson", "encoding": "UTF-8", "content": "x", "hasMore": false}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCompactInstance(t, test.schema, test.instance); err != nil {
				t.Fatal(err)
			}
		})
	}

	missingCore := map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "items": []any{}, "hasMore": false}}
	if err := validateCompactInstance(t, frameQueryOutputSchema(), missingCore); err == nil {
		t.Fatal("frame projection omission was accepted")
	}
	for name, instance := range map[string]any{
		"neither record mode":    map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "hasMore": false}},
		"both record modes":      map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "items": []any{}, "matches": []any{}, "contentDescriptors": []any{}, "search": map[string]any{}, "hasMore": false}},
		"incomplete search mode": map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "matches": []any{}, "hasMore": false}},
		"descriptor missing id":  map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "matches": []any{}, "contentDescriptors": []any{map[string]any{"contentRef": "opaque"}}, "search": map[string]any{}, "hasMore": false}},
		"descriptor missing ref": map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "matches": []any{}, "contentDescriptors": []any{map[string]any{"contentId": "c1"}}, "search": map[string]any{}, "hasMore": false}},
	} {
		if err := validateCompactInstance(t, recordQueryOutputSchema(), instance); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestActiveCompactSchemasExposeFactsAndNoDerivedDiagnosticStates(t *testing.T) {
	execution := compactExecutionSchema()
	for _, name := range []string{"sessionId", "traceId", "lastCanonicalSequence", "startedAt", "updatedAt", "elapsedMillis", "entrySkill", "status", "phase", "summary", "activePath", "totalFrameDepth", "activePathTruncated", "usage", "configuredLimits"} {
		if execution.Properties[name] == nil {
			t.Errorf("execution schema does not advertise %q", name)
		}
	}
	for _, name := range []string{"skillInvocations", "toolInvocations", "linterRetries", "modelCalls", "providerAttempts", "promptUnits", "completionUnits", "usageUnits", "exactModelResponses", "heuristicModelResponses", "unavailableModelResponses"} {
		if execution.Properties["usage"].Properties[name] == nil {
			t.Errorf("usage schema does not advertise %q", name)
		}
	}
	for _, name := range []string{"maxSkillInvocations", "maxToolInvocations", "maxLinterRetries", "maxModelCalls", "maxProviderAttempts", "maxUsageUnits"} {
		if execution.Properties["configuredLimits"].Properties[name] == nil {
			t.Errorf("configured-limits schema does not advertise %q", name)
		}
	}
	activity := activityOutputSchema().Properties["result"]
	for _, name := range []string{"returnedCursorRange", "continuity", "coverage"} {
		if activity.Properties[name] == nil {
			t.Errorf("activity schema does not advertise %q", name)
		}
	}
	coverage := activity.Properties["coverage"]
	for _, name := range []string{"globalEvictedThroughCursor", "sessionStartCursor", "sessionEvictedThroughCursor", "sessionRetainedCursorRange"} {
		if coverage.Properties[name] == nil {
			t.Errorf("coverage schema does not advertise %q", name)
		}
	}
	for _, forbidden := range []string{"beginningUnavailable", "complete", "incomplete", "unknown", "progress", "health", "stuck", "diagnosis", "recommendation"} {
		if activity.Properties[forbidden] != nil || coverage.Properties[forbidden] != nil {
			t.Errorf("active discovery advertises derived field %q", forbidden)
		}
	}
}

func TestTraceSummaryHistogramSchemaRejectsUnknownAndInvalidCounts(t *testing.T) {
	base := map[string]any{"result": map[string]any{"evidence": compactEvidenceInstance(), "summary": map[string]any{"outcome": "SUCCEEDED", "recordCount": 1, "recordCountsByType": map[string]any{"TRACE_STARTED": 1}, "frameCount": 0, "attemptCount": 0, "retryCount": 0, "failureCount": 0, "rootFrameIds": []any{}, "usageComplete": true}}}
	resolved, err := traceSummaryOutputSchema().Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolved.Validate(base); err != nil {
		t.Fatalf("valid histogram rejected: %v", err)
	}
	for name, counts := range map[string]map[string]any{
		"unknown":  {"UNKNOWN": 1},
		"negative": {"TRACE_STARTED": -1},
		"fraction": {"TRACE_STARTED": 1.5},
		"string":   {"TRACE_STARTED": "1"},
	} {
		candidate, _ := json.Marshal(base)
		var instance map[string]any
		_ = json.Unmarshal(candidate, &instance)
		instance["result"].(map[string]any)["summary"].(map[string]any)["recordCountsByType"] = counts
		if err := resolved.Validate(instance); err == nil {
			t.Fatalf("%s histogram accepted", name)
		}
		if err := validateCompleteRecordCounts(instance); err == nil {
			t.Fatalf("complete validator accepted %s histogram", name)
		}
	}
}

func compactEvidenceInstance() map[string]any {
	return map[string]any{"traceId": "trace", "sessionId": "session", "observedAt": "2026-08-19T00:00:00Z"}
}
