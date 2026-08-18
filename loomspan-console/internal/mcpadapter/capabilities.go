package mcpadapter

type capabilityDescriptor struct {
	ID                       string   `json:"id"`
	RequiredTools            []string `json:"requiredTools"`
	RequiredSemanticFixtures []string `json:"requiredSemanticFixtures,omitempty"`
}

const (
	SkillInspectionCapability           = "loomspan.skill-inspection.v1"
	ActiveExecutionInspectionCapability = "loomspan.active-execution-inspection.v1"
	RecentActivityInspectionCapability  = "loomspan.recent-activity-inspection.v1"
	TraceInspectionCapability           = "loomspan.trace-inspection.v1"
	RawArtifactInspectionCapability     = "loomspan.raw-artifact-inspection.v1"
)

var traceSemanticFixtures = []string{
	"trace.target-acquisition", "trace.target-free-import", "trace.trace-id-resolution", "trace.ambiguous-identity", "trace.discovery-completeness",
	"trace.parity", "trace.fact-projection", "trace.continuation", "trace.lifecycle", "trace.cancellation",
	"trace.unavailable-evidence", "trace.concurrent-clients", "trace.joined-adapters", "trace.schema-errors",
}
var rawArtifactSemanticFixtures = []string{"raw.exact-range", "raw.trace-id-continuation", "raw.lifecycle-errors", "raw.resolver-only", "raw.inert-content"}

var capabilityDescriptors = []capabilityDescriptor{
	{ID: SkillInspectionCapability, RequiredTools: []string{ListSkillsToolName, GetSkillToolName}},
	{ID: ActiveExecutionInspectionCapability, RequiredTools: []string{ListExecutionsToolName, GetExecutionToolName}},
	{ID: RecentActivityInspectionCapability, RequiredTools: []string{GetExecutionActivityToolName}},
	{ID: TraceInspectionCapability, RequiredTools: []string{ListTracesToolName, GetTraceToolName, QueryTraceFramesToolName, QueryTraceRecordsToolName, ReadTracePayloadToolName}, RequiredSemanticFixtures: traceSemanticFixtures},
	{ID: RawArtifactInspectionCapability, RequiredTools: []string{ReadTraceArtifactToolName}, RequiredSemanticFixtures: rawArtifactSemanticFixtures},
}

func installedCapabilities() []string {
	tools := map[string]bool{}
	for _, name := range []string{ListSkillsToolName, GetSkillToolName, ListExecutionsToolName, GetExecutionToolName, GetExecutionActivityToolName, ListTracesToolName, GetTraceToolName, QueryTraceFramesToolName, QueryTraceRecordsToolName, ReadTracePayloadToolName, ReadTraceArtifactToolName} {
		tools[name] = true
	}
	return capabilitiesWithCompleteTools(tools)
}

// capabilitiesWithCompleteTools is the production assembly rule. Semantic
// completeness is a build invariant proved by the reviewed manifest runner;
// runtime data or a manifest declaration never self-attests that behavior.
func capabilitiesWithCompleteTools(tools map[string]bool) []string {
	capabilities := make([]string, 0, len(capabilityDescriptors)+1)
	capabilities = append(capabilities, RuntimeStatusCapability)
	for _, descriptor := range capabilityDescriptors {
		complete := true
		for _, tool := range descriptor.RequiredTools {
			complete = complete && tools[tool]
		}
		if complete {
			capabilities = append(capabilities, descriptor.ID)
		}
	}
	return capabilities
}

func conformantCapabilities(tools, fixtures map[string]bool) []string {
	capabilities := make([]string, 0, len(capabilityDescriptors)+1)
	capabilities = append(capabilities, RuntimeStatusCapability)
	for _, descriptor := range capabilityDescriptors {
		complete := true
		for _, tool := range descriptor.RequiredTools {
			complete = complete && tools[tool]
		}
		for _, fixture := range descriptor.RequiredSemanticFixtures {
			complete = complete && fixtures[fixture]
		}
		if !complete {
			continue
		}
		capabilities = append(capabilities, descriptor.ID)
	}
	return capabilities
}
