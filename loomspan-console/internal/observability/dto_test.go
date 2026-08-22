package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceStatusDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "instance-status.json")
	var status InstanceStatus
	if err := json.Unmarshal(body, &status); err != nil {
		t.Fatal(err)
	}
	if status.InstanceID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected instanceId: %s", status.InstanceID)
	}
	if !status.LiveMonitoringAvailable {
		t.Fatal("expected liveMonitoringAvailable=true")
	}
	if status.RegisteredSkillCount != 1 {
		t.Fatalf("unexpected registeredSkillCount: %d", status.RegisteredSkillCount)
	}
}

func TestSkillPageDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "skills-page.json")
	var page Page[SkillSummary]
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].RegisteredName != "CheckDns" {
		t.Fatalf("unexpected registeredName: %s", page.Items[0].RegisteredName)
	}
	if page.HasMore {
		t.Fatal("expected hasMore=false")
	}
	if page.NextCursor != nil {
		t.Fatalf("expected nextCursor=null, got %v", page.NextCursor)
	}
}

func TestSkillDetailDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "skill-detail.json")
	var detail SkillDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.RegisteredName != "CheckDns" {
		t.Fatalf("unexpected registeredName: %s", detail.RegisteredName)
	}
	if detail.Yaml == "" {
		t.Fatal("expected non-empty yaml")
	}
}

func TestActivePageDecodesFromFixtureWithResumeCursor(t *testing.T) {
	body := readFixture(t, "active-executions-page.json")
	var page ActivePage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].SessionID != "session-1" {
		t.Fatalf("unexpected sessionId: %s", page.Items[0].SessionID)
	}
	if page.ResumeCursor == nil || *page.ResumeCursor != "9" {
		t.Fatalf("expected resumeCursor=9, got %v", page.ResumeCursor)
	}
}

func TestActiveExecutionDetailDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "active-execution-detail.json")
	var exec ActiveExecution
	if err := json.Unmarshal(body, &exec); err != nil {
		t.Fatal(err)
	}
	if exec.SessionID != "session-1" {
		t.Fatalf("unexpected sessionId: %s", exec.SessionID)
	}
	if exec.EntrySkill != "CheckDns" {
		t.Fatalf("unexpected entrySkill: %s", exec.EntrySkill)
	}
	if exec.Usage.SkillInvocations != 1 {
		t.Fatalf("unexpected skillInvocations: %d", exec.Usage.SkillInvocations)
	}
}

func TestActiveExecutionRequiresEveryUsageAndConfiguredLimitMember(t *testing.T) {
	body := readFixture(t, "active-execution-detail.json")
	for objectName, names := range map[string][]string{
		"usage":            requiredUsageMembers,
		"configuredLimits": requiredLimitMembers,
	} {
		for _, name := range names {
			t.Run(objectName+"/"+name, func(t *testing.T) {
				var execution map[string]json.RawMessage
				if err := json.Unmarshal(body, &execution); err != nil {
					t.Fatal(err)
				}
				var members map[string]json.RawMessage
				if err := json.Unmarshal(execution[objectName], &members); err != nil {
					t.Fatal(err)
				}
				delete(members, name)
				execution[objectName], _ = json.Marshal(members)
				mutated, _ := json.Marshal(execution)
				if err := validateActiveExecutionJSON(mutated); err == nil || !strings.Contains(err.Error(), objectName+"."+name+" is missing") {
					t.Fatalf("missing member error = %v", err)
				}
			})
		}
	}
}

func TestActiveExecutionPreservesObservedProviderZeroAndDisabledLimit(t *testing.T) {
	body := readFixture(t, "active-execution-detail.json")
	var execution map[string]json.RawMessage
	if err := json.Unmarshal(body, &execution); err != nil {
		t.Fatal(err)
	}
	for objectName, memberName := range map[string]string{"usage": "providerAttempts", "configuredLimits": "maxProviderAttempts"} {
		var members map[string]json.RawMessage
		if err := json.Unmarshal(execution[objectName], &members); err != nil {
			t.Fatal(err)
		}
		members[memberName] = json.RawMessage("0")
		execution[objectName], _ = json.Marshal(members)
	}
	mutated, _ := json.Marshal(execution)
	if err := validateActiveExecutionJSON(mutated); err != nil {
		t.Fatal(err)
	}
	var decoded ActiveExecution
	if err := json.Unmarshal(mutated, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Usage.ProviderAttempts != 0 || decoded.ConfiguredLimits.MaxProviderAttempts != 0 {
		t.Fatalf("explicit zeros changed: usage=%d limit=%d", decoded.Usage.ProviderAttempts, decoded.ConfiguredLimits.MaxProviderAttempts)
	}
}

func TestActiveExecutionDecodesCanonicalFramePathFields(t *testing.T) {
	body := []byte(`{
		"sessionId":"session-1",
		"traceId":"trace-1",
		"startedAt":"2026-07-25T11:59:55Z",
		"updatedAt":"2026-07-25T11:59:59Z",
		"activePath":[{"frameId":"frame-1","frameType":"SKILL_EXECUTION","route":"CheckDns"}]
	}`)
	var execution ActiveExecution
	if err := json.Unmarshal(body, &execution); err != nil {
		t.Fatal(err)
	}
	if len(execution.ActivePath) != 1 {
		t.Fatalf("expected one active-path entry, got %d", len(execution.ActivePath))
	}
	entry := execution.ActivePath[0]
	if entry.FrameID != "frame-1" || entry.FrameType != "SKILL_EXECUTION" || entry.Route != "CheckDns" {
		t.Fatalf("unexpected active-path entry: %#v", entry)
	}
	encoded, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"frameId":"frame-1"`, `"frameType":"SKILL_EXECUTION"`, `"route":"CheckDns"`} {
		if !json.Valid(encoded) || !containsJSONField(encoded, field) {
			t.Fatalf("encoded active execution does not preserve %s: %s", field, encoded)
		}
	}
}

func TestTracePageDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "traces-page.json")
	var page Page[Trace]
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].TraceID != "trace-1" {
		t.Fatalf("unexpected traceId: %s", page.Items[0].TraceID)
	}
}

func TestTraceDetailDecodesFromFixture(t *testing.T) {
	body := readFixture(t, "trace-detail.json")
	var trace Trace
	if err := json.Unmarshal(body, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.TraceID != "trace-1" {
		t.Fatalf("unexpected traceId: %s", trace.TraceID)
	}
	if trace.Outcome != "SUCCEEDED" {
		t.Fatalf("unexpected outcome: %s", trace.Outcome)
	}
}

func TestContinuationPageDecodesWithCursor(t *testing.T) {
	body := readFixture(t, "continuation-page.json")
	var page Page[SkillSummary]
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if !page.HasMore {
		t.Fatal("expected hasMore=true")
	}
	if page.NextCursor == nil {
		t.Fatal("expected non-null nextCursor")
	}
}

func TestEmptyPageDecodesWithZeroItems(t *testing.T) {
	body := readFixture(t, "empty-page.json")
	var page Page[SkillSummary]
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(page.Items))
	}
	if page.HasMore {
		t.Fatal("expected hasMore=false")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", name))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func containsJSONField(body []byte, field string) bool {
	return string(body) != "" && len(field) > 0 &&
		strings.Contains(string(body), field)
}
