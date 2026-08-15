package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/agentskills"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/webassets"
)

type realRunner struct{}

func (realRunner) run(current phase, context pipelineContext) error {
	switch current {
	case phaseToolchains:
		goVersion, err := commandOutput(context.paths.module, nil, "go", "version")
		if err != nil {
			return err
		}
		nodeVersion, err := commandOutput(context.paths.module, nil, "node", "--version")
		if err != nil {
			return err
		}
		npmVersion, err := commandOutput(context.paths.module, nil, "npm", "--version")
		if err != nil {
			return err
		}
		return validateToolchainVersions(goVersion, nodeVersion, npmVersion)
	case phaseNPMCI:
		return runCommand(context.paths.web, nil, "npm", "ci", "--allow-remote=all")
	case phaseAgentSkill:
		return agentskills.ValidateRuntimeDebugging(filepath.Join(context.paths.agentSkills, agentskills.RuntimeDebuggingSkillName))
	case phaseFrontendTypecheck:
		return runCommand(context.paths.web, nil, "npm", "run", "typecheck")
	case phaseFrontendCoverage:
		return runCommand(context.paths.web, nil, "npm", "run", "test:coverage")
	case phaseCleanAssets:
		return cleanGeneratedAssets(context.paths)
	case phaseViteBuild:
		return runCommand(context.paths.web, nil, "npm", "run", "build:web")
	case phaseGenerateManifest:
		return generateManifest(context.paths.generated, context.productVersion)
	case phaseVerifyManifest:
		_, err := webassets.Verify(os.DirFS(context.paths.generated), context.productVersion)
		return err
	case phaseGoTests:
		return runCommand(context.paths.module, nil, "go", "test", "./...")
	case phaseGoBuild:
		if err := os.MkdirAll(context.paths.build, 0o755); err != nil {
			return err
		}
		output := filepath.Join(context.paths.build, executableName())
		ldflag := "-X=github.com/mgiacomi/loomspan/loomspan-console/internal/release.productVersion=" + context.productVersion
		return runCommand(context.paths.module, nil, "go", "build", "-trimpath", "-ldflags", ldflag, "-o", output, "./cmd/loomspan-console")
	default:
		return fmt.Errorf("unknown pipeline phase %q", current)
	}
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "loomspan-console.exe"
	}
	return "loomspan-console"
}

func commandOutput(directory string, environment []string, name string, arguments ...string) (string, error) {
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %v failed: %w\n%s", name, arguments, err, output)
	}
	return string(output), nil
}

func runCommand(directory string, environment []string, name string, arguments ...string) error {
	command := exec.Command(name, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %v failed: %w", name, arguments, err)
	}
	return nil
}
