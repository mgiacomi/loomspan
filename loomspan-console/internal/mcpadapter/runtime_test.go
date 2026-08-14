package mcpadapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
)

func TestRuntimeOutputGoldenAndTextAgree(t *testing.T) {
	output := RuntimeOutput{
		Capabilities: installedCapabilities(),
		Status:       consolecore.NoTargetStatus(time.Unix(1, 0).UTC()),
	}
	actual, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "runtime-no-target.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(actual)+"\n" != string(expected) {
		t.Fatalf("runtime output changed:\n%s", actual)
	}
	text := runtimeText(output)
	for _, required := range []string{
		"capability: " + RuntimeStatusCapability,
		"capability: " + SkillInspectionCapability,
		"capability: " + ActiveExecutionInspectionCapability,
		"capability: " + RecentActivityInspectionCapability,
		"targetSelection: NONE",
		"targetConnection: NOT_APPLICABLE",
		"observedAt: 1970-01-01T00:00:01Z",
	} {
		if !containsLine(text, required) {
			t.Errorf("runtime text does not contain %q: %s", required, text)
		}
	}
}

func TestRuntimeOutputSucceedsForEveryTargetStatusFactAndRejectsInvalidInvariant(t *testing.T) {
	observed := time.Unix(2, 0).UTC()
	statuses := map[string]consolecore.StatusSnapshot{
		"no target": consolecore.NoTargetStatus(observed),
		"disconnected": {
			ObservedAt: observed, TargetScopeID: "scope-disconnected", TargetSelection: consolecore.SelectionSelected,
			TargetConnection: consolecore.ConnectionUnavailable, TargetAuthentication: consolecore.AuthenticationUnknown,
			JavaGoCompatibility: consolecore.CompatibilityNotChecked, RuntimeIdentity: consolecore.RuntimeNotEstablished, LiveMonitoring: consolecore.LiveUnknown,
		},
		"authentication required": {
			ObservedAt: observed, TargetScopeID: "scope-auth", TargetSelection: consolecore.SelectionSelected,
			TargetConnection: consolecore.ConnectionReachable, TargetAuthentication: consolecore.AuthenticationRequired,
			JavaGoCompatibility: consolecore.CompatibilityNotChecked, RuntimeIdentity: consolecore.RuntimeNotEstablished, LiveMonitoring: consolecore.LiveUnknown,
		},
		"incompatible": {
			ObservedAt: observed, TargetScopeID: "scope-incompatible", TargetSelection: consolecore.SelectionSelected,
			TargetConnection: consolecore.ConnectionReachable, TargetAuthentication: consolecore.AuthenticationEstablished,
			JavaGoCompatibility: consolecore.CompatibilityIncompatible, RuntimeIdentity: consolecore.RuntimeEstablished, InstanceID: "instance-incompatible", LiveMonitoring: consolecore.LiveUnavailable,
		},
		"connected": {
			ObservedAt: observed, TargetScopeID: "scope-connected", TargetSelection: consolecore.SelectionSelected,
			TargetConnection: consolecore.ConnectionReachable, TargetAuthentication: consolecore.AuthenticationEstablished,
			JavaGoCompatibility: consolecore.CompatibilityCompatible, RuntimeIdentity: consolecore.RuntimeEstablished, InstanceID: "instance-connected", LiveMonitoring: consolecore.LiveAvailable,
		},
	}
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}, key: "secret"}
	for name, status := range statuses {
		t.Run(name, func(t *testing.T) {
			output, err := buildRuntimeOutput(context.Background(), func() consolecore.StatusSnapshot { return status }, credentials)
			if err != nil || len(output.Capabilities) != 6 || output.Capabilities[0] != RuntimeStatusCapability || output.Status != status {
				t.Fatalf("output=%+v err=%v", output, err)
			}
		})
	}
	invalid := consolecore.NoTargetStatus(time.Time{})
	if _, err := buildRuntimeOutput(context.Background(), func() consolecore.StatusSnapshot { return invalid }, credentials); err == nil || err.Error() != "INTERNAL: runtime status is unavailable" {
		t.Fatalf("invalid status error = %v", err)
	}
}

func containsLine(text, line string) bool {
	for _, candidate := range splitLines(text) {
		if candidate == line {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	var lines []string
	for len(text) > 0 {
		index := 0
		for index < len(text) && text[index] != '\n' {
			index++
		}
		lines = append(lines, text[:index])
		if index == len(text) {
			break
		}
		text = text[index+1:]
	}
	return lines
}
