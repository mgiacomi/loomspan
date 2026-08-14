package mcpadapter

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRuntimeCapabilitiesMatchCompleteFamilies(t *testing.T) {
	capabilities := installedCapabilities()
	if len(capabilities) != 6 || capabilities[0] != RuntimeStatusCapability {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	want := map[string]bool{TraceInspectionCapability: true, RawArtifactInspectionCapability: true}
	for _, capability := range capabilities {
		delete(want, capability)
	}
	if len(want) != 0 {
		t.Fatalf("missing advertised capabilities: %#v", want)
	}
}

func TestCapabilityDescriptorsMatchInstalledToolFamilies(t *testing.T) {
	want := []capabilityDescriptor{
		{ID: SkillInspectionCapability, RequiredTools: []string{ListSkillsToolName, GetSkillToolName}},
		{ID: ActiveExecutionInspectionCapability, RequiredTools: []string{ListExecutionsToolName, GetExecutionToolName}},
		{ID: RecentActivityInspectionCapability, RequiredTools: []string{GetExecutionActivityToolName}},
		{ID: TraceInspectionCapability, RequiredTools: []string{ListTracesToolName, GetTraceToolName, QueryTraceFramesToolName, QueryTraceRecordsToolName, ReadTracePayloadToolName}, RequiredSemanticFixtures: traceSemanticFixtures},
		{ID: RawArtifactInspectionCapability, RequiredTools: []string{ReadTraceArtifactToolName}, RequiredSemanticFixtures: rawArtifactSemanticFixtures},
	}
	if !reflect.DeepEqual(capabilityDescriptors, want) {
		t.Fatalf("capability descriptors = %#v, want %#v", capabilityDescriptors, want)
	}
}

func TestPR18CapabilityManifestMatchesReviewedDescriptorAndRejectsIndependentGaps(t *testing.T) {
	body, err := os.ReadFile("contracts/trace-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var reviewed []capabilityDescriptor
	if err := json.Unmarshal(body, &reviewed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reviewed, capabilityDescriptors[len(capabilityDescriptors)-2:]) {
		t.Fatalf("reviewed manifest=%#v descriptors=%#v", reviewed, capabilityDescriptors)
	}
	tools := map[string]bool{}
	fixtures := map[string]bool{}
	for _, descriptor := range reviewed {
		for _, tool := range descriptor.RequiredTools {
			tools[tool] = true
		}
		for _, fixture := range descriptor.RequiredSemanticFixtures {
			fixtures[fixture] = true
		}
	}
	for _, descriptor := range reviewed {
		for _, tool := range descriptor.RequiredTools {
			delete(tools, tool)
			if got := conformantCapabilities(tools, fixtures); containsCapability(got, descriptor.ID) {
				t.Fatalf("capability %s survived missing tool %s: %#v", descriptor.ID, tool, got)
			}
			tools[tool] = true
		}
		for _, fixture := range descriptor.RequiredSemanticFixtures {
			delete(fixtures, fixture)
			if got := conformantCapabilities(tools, fixtures); containsCapability(got, descriptor.ID) {
				t.Fatalf("capability %s survived missing fixture %s: %#v", descriptor.ID, fixture, got)
			}
			fixtures[fixture] = true
		}
	}
}

func TestPR18CapabilityManifestRequiredToolsAreRegistered(t *testing.T) {
	options := newMCPTestOptions(t, nil)
	server := NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	session := connectMCPTestSession(t, httpServer.URL, "mcp-secret")
	defer session.Close()
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registered := map[string]bool{}
	for _, tool := range listed.Tools {
		registered[tool.Name] = true
	}
	for _, descriptor := range capabilityDescriptors[len(capabilityDescriptors)-2:] {
		for _, required := range descriptor.RequiredTools {
			if !registered[required] {
				t.Fatalf("capability %s requires unregistered tool %s", descriptor.ID, required)
			}
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: RuntimeToolName, Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("runtime=%#v err=%v", result, err)
	}
}

func containsCapability(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
