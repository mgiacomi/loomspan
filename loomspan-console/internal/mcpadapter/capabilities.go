package mcpadapter

type capabilityDescriptor struct {
	ID            string
	RequiredTools []string
}

const (
	SkillInspectionCapability           = "loomspan.skill-inspection.v1"
	ActiveExecutionInspectionCapability = "loomspan.active-execution-inspection.v1"
	RecentActivityInspectionCapability  = "loomspan.recent-activity-inspection.v1"
)

var capabilityDescriptors = []capabilityDescriptor{
	{ID: SkillInspectionCapability, RequiredTools: []string{ListSkillsToolName, GetSkillToolName}},
	{ID: ActiveExecutionInspectionCapability, RequiredTools: []string{ListExecutionsToolName, GetExecutionToolName}},
	{ID: RecentActivityInspectionCapability, RequiredTools: []string{GetExecutionActivityToolName}},
}

func installedCapabilities() []string {
	capabilities := make([]string, 0, len(capabilityDescriptors)+1)
	capabilities = append(capabilities, RuntimeStatusCapability)
	for _, descriptor := range capabilityDescriptors {
		capabilities = append(capabilities, descriptor.ID)
	}
	return capabilities
}
