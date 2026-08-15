package main

import "fmt"

type buildMode string

const (
	modeVerify  buildMode = "verify"
	modeBuild   buildMode = "build"
	modePackage buildMode = "package"
	modeSmoke   buildMode = "smoke"
)

type phase string

const (
	phaseToolchains        phase = "verify toolchains"
	phaseNPMCI             phase = "install locked frontend dependencies"
	phaseAgentSkill        phase = "validate runtime debugging Agent Skill"
	phaseFrontendTypecheck phase = "type-check frontend"
	phaseFrontendCoverage  phase = "test frontend with coverage"
	phaseCleanAssets       phase = "clean generated assets"
	phaseViteBuild         phase = "build frontend assets"
	phaseGenerateManifest  phase = "generate asset manifest"
	phaseVerifyManifest    phase = "verify asset manifest"
	phaseGoTests           phase = "test Go module"
	phaseGoBuild           phase = "build executable"
)

type pipelineContext struct {
	paths          projectPaths
	productVersion string
}

type pipelineDependencies struct {
	run func(phase, pipelineContext) error
}

func runPipeline(mode buildMode, context pipelineContext, dependencies pipelineDependencies) error {
	phases := []phase{
		phaseToolchains,
		phaseNPMCI,
		phaseAgentSkill,
		phaseFrontendTypecheck,
		phaseFrontendCoverage,
		phaseCleanAssets,
		phaseViteBuild,
		phaseGenerateManifest,
		phaseVerifyManifest,
		phaseGoTests,
	}
	if mode == modeBuild || mode == modePackage {
		phases = append(phases, phaseGoBuild)
	}
	for _, current := range phases {
		if err := dependencies.run(current, context); err != nil {
			return fmt.Errorf("%s: %w", current, err)
		}
	}
	return nil
}
