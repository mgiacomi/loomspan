package mcpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolEnvelopeContainsExactlyOneResultOrError(t *testing.T) {
	success, envelope, err := successResult(struct {
		Value string `json:"value"`
	}{Value: "ok"}, "ok\n")
	if err != nil || success.IsError || envelope.Result == nil || envelope.Error != nil {
		t.Fatalf("success result=%#v envelope=%#v err=%v", success, envelope, err)
	}
	failure, failedEnvelope, err := domainFailure[struct{}](consolecore.NewError(consolecore.CodeNotFound, "Not found.", "scope-1", consolecore.Details{}, nil))
	if err != nil || !failure.IsError || failedEnvelope.Result != nil || failedEnvelope.Error == nil {
		t.Fatalf("failure result=%#v envelope=%#v err=%v", failure, failedEnvelope, err)
	}
}

func TestDomainErrorResultIsStructuredMarkedAndSafe(t *testing.T) {
	domain := consolecore.NewError(consolecore.CodeConsoleError, "Safe message.", "scope-1", consolecore.Details{
		ExpectedCompatibilityVersion: "expected", ObservedCompatibilityVersion: "observed",
		CurrentTargetScopeID: "scope-2", TransportCategory: "protocol", LimitName: "items", LimitValue: 64,
	}, errors.New("SECRET internal cause C:\\private\\file"))
	result, envelope, err := domainFailure[struct{}](domain)
	if err != nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if text != "CONSOLE_ERROR: Safe message." || strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "private") {
		t.Fatalf("text=%q encoded=%s", text, encoded)
	}
	if !strings.Contains(string(encoded), `"details":{`) || strings.Contains(string(encoded), `"details":null`) {
		t.Fatalf("details are not an object: %s", encoded)
	}
}

func TestPR17TextFallbacksHaveExactOrderEscapingAndFinalNewline(t *testing.T) {
	result := skillListResult{
		TargetScopeID: "scope\n1", InstanceID: "instance:1",
		ObservedAt: time.Date(2026, 8, 13, 20, 0, 0, 123, time.FixedZone("offset", -7*60*60)),
		Items:      []skillSummaryDTO{{RegisteredName: "name\nquoted\"", SourcePath: "skills/a.yaml", ResourceURI: "loomspan://targets/scope/skills/name"}},
	}
	text := skillListText(result)
	wantPrefix := "targetScopeId: \"scope\\n1\"\ninstanceId: \"instance:1\"\nobservedAt: \"2026-08-14T03:00:00.000000123Z\"\ncount: 1\nhasMore: false\ncontinuation: -\n"
	if !strings.HasPrefix(text, wantPrefix) || !strings.HasSuffix(text, "\n") || strings.Count(text, "name\\nquoted") != 1 {
		t.Fatalf("text fallback changed:\n%s", text)
	}
}

func TestPR17TextFallbacksUseJSONEscapesForControlCharacters(t *testing.T) {
	var writer lineWriter
	writer.quoted("value", "nul:\x00 unit-separator:\x1f newline:\n")
	want := "value: \"nul:\\u0000 unit-separator:\\u001f newline:\\n\"\n"
	if got := writer.String(); got != want {
		t.Fatalf("text fallback quoting = %q, want %q", got, want)
	}
	if strings.Contains(writer.String(), `\x`) {
		t.Fatalf("text fallback contains a non-JSON escape: %q", writer.String())
	}
	for control := rune(0); control <= 0x1f; control++ {
		var controlWriter lineWriter
		value := string(control)
		controlWriter.quoted("value", value)
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := controlWriter.String(), "value: "+string(encoded)+"\n"; got != want || strings.Contains(got, `\x`) {
			t.Fatalf("control U+%04X quoting = %q, want %q", control, got, want)
		}
	}
}

func TestMCPToolsUseReadOnlyClosedWorldAnnotations(t *testing.T) {
	if readOnlyAnnotations == nil || !readOnlyAnnotations.ReadOnlyHint || !readOnlyAnnotations.IdempotentHint ||
		readOnlyAnnotations.DestructiveHint == nil || *readOnlyAnnotations.DestructiveHint ||
		readOnlyAnnotations.OpenWorldHint == nil || *readOnlyAnnotations.OpenWorldHint {
		t.Fatalf("annotations = %#v", readOnlyAnnotations)
	}
}

func TestCheckedDomainFailureSuppressesChangedAuthenticationGeneration(t *testing.T) {
	ctx := context.WithValue(context.Background(), generationKey{}, uint64(1))
	options := ServerOptions{Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 2}}}
	result, _, err := checkedDomainFailure[struct{}](ctx, options, consolecore.NewError(consolecore.CodeNotFound, "Not found.", "", consolecore.Details{}, nil))
	if err == nil || result != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMCPGoldenInventoryContainsOnlyImplementedSurface(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"runtime-no-target.json": true, "skills-list.json": true, "skill-detail.json": true,
		"executions-list.json": true, "execution-detail.json": true, "activity.json": true,
	}
	if len(entries) != len(want) {
		t.Fatalf("golden entries = %v", entries)
	}
	for _, entry := range entries {
		if entry.IsDir() || !want[entry.Name()] {
			t.Fatalf("unexpected golden entry %q", entry.Name())
		}
	}
}
