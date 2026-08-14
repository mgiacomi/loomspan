package mcpadapter

import "testing"

func TestRuntimeCapabilitiesMatchCompletePR17ToolFamilies(t *testing.T) {
	want := []capabilityDescriptor{
		{SkillInspectionCapability, []string{ListSkillsToolName, GetSkillToolName}},
		{ActiveExecutionInspectionCapability, []string{ListExecutionsToolName, GetExecutionToolName}},
		{RecentActivityInspectionCapability, []string{GetExecutionActivityToolName}},
	}
	if len(capabilityDescriptors) != len(want) {
		t.Fatalf("descriptors = %#v", capabilityDescriptors)
	}
	for index := range want {
		if capabilityDescriptors[index].ID != want[index].ID || len(capabilityDescriptors[index].RequiredTools) != len(want[index].RequiredTools) {
			t.Fatalf("descriptor[%d] = %#v", index, capabilityDescriptors[index])
		}
		for toolIndex := range want[index].RequiredTools {
			if capabilityDescriptors[index].RequiredTools[toolIndex] != want[index].RequiredTools[toolIndex] {
				t.Fatalf("descriptor[%d] = %#v", index, capabilityDescriptors[index])
			}
		}
	}
	capabilities := installedCapabilities()
	if len(capabilities) != 4 || capabilities[0] != RuntimeStatusCapability {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

func TestCapabilityConformanceRejectsEveryMissingRequiredTool(t *testing.T) {
	all := map[string]bool{}
	for _, descriptor := range capabilityDescriptors {
		for _, tool := range descriptor.RequiredTools {
			all[tool] = true
		}
	}
	for missing := range all {
		installed := map[string]bool{}
		for tool := range all {
			if tool != missing {
				installed[tool] = true
			}
		}
		if capabilityFamiliesComplete(installed) {
			t.Fatalf("missing %s was accepted", missing)
		}
	}
}

func capabilityFamiliesComplete(installed map[string]bool) bool {
	for _, descriptor := range capabilityDescriptors {
		for _, tool := range descriptor.RequiredTools {
			if !installed[tool] {
				return false
			}
		}
	}
	return true
}
