package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestRunPipelineStopsWhenFrontendTestsFail(t *testing.T) {
	frontendFailure := errors.New("frontend coverage failed")
	var calls []phase
	deps := pipelineDependencies{
		run: func(current phase, _ pipelineContext) error {
			calls = append(calls, current)
			if current == phaseFrontendCoverage {
				return frontendFailure
			}
			return nil
		},
	}

	err := runPipeline(modeVerify, pipelineContext{productVersion: "0.1.0-SNAPSHOT"}, deps)

	if !errors.Is(err, frontendFailure) {
		t.Fatalf("runPipeline() error = %v, want %v", err, frontendFailure)
	}
	want := []phase{
		phaseToolchains,
		phaseNPMCI,
		phaseAgentSkill,
		phaseFrontendTypecheck,
		phaseFrontendCoverage,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("phases = %v, want %v", calls, want)
	}
}

func TestRunPipelineExecutesRequiredOrder(t *testing.T) {
	var calls []phase
	deps := pipelineDependencies{run: func(current phase, _ pipelineContext) error {
		calls = append(calls, current)
		return nil
	}}
	if err := runPipeline(modeBuild, pipelineContext{}, deps); err != nil {
		t.Fatal(err)
	}
	want := []phase{
		phaseToolchains, phaseNPMCI, phaseAgentSkill, phaseFrontendTypecheck, phaseFrontendCoverage,
		phaseCleanAssets, phaseViteBuild, phaseGenerateManifest, phaseVerifyManifest,
		phaseGoTests, phaseGoBuild,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("phases = %v, want %v", calls, want)
	}
}

func TestRunPipelineStopsAtEveryFailedPhase(t *testing.T) {
	all := []phase{
		phaseToolchains, phaseNPMCI, phaseAgentSkill, phaseFrontendTypecheck, phaseFrontendCoverage,
		phaseCleanAssets, phaseViteBuild, phaseGenerateManifest, phaseVerifyManifest,
		phaseGoTests, phaseGoBuild,
	}
	for failureIndex, failing := range all {
		t.Run(string(failing), func(t *testing.T) {
			var calls []phase
			sentinel := errors.New("sentinel")
			deps := pipelineDependencies{run: func(current phase, _ pipelineContext) error {
				calls = append(calls, current)
				if current == failing {
					return sentinel
				}
				return nil
			}}
			err := runPipeline(modeBuild, pipelineContext{}, deps)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(calls, all[:failureIndex+1]) {
				t.Fatalf("phases = %v, want %v", calls, all[:failureIndex+1])
			}
		})
	}
}

func TestVerifyModeDoesNotBuildBinary(t *testing.T) {
	var calls []phase
	err := runPipeline(modeVerify, pipelineContext{}, pipelineDependencies{run: func(current phase, _ pipelineContext) error {
		calls = append(calls, current)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range calls {
		if current == phaseGoBuild {
			t.Fatal("verify mode requested a binary build")
		}
	}
}
