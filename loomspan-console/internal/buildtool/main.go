package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "loomspan-console build:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) > 0 && arguments[0] == "mcp-conformance" {
		if len(arguments) != 1 {
			return fmt.Errorf("mcp-conformance accepts no arguments")
		}
		paths, err := resolveProjectPaths()
		if err != nil {
			return err
		}
		return runMCPConformance(paths)
	}
	if len(arguments) == 0 || (arguments[0] != string(modeVerify) && arguments[0] != string(modeBuild) && arguments[0] != string(modePackage) && arguments[0] != string(modeSmoke)) {
		return fmt.Errorf("usage: go run ./internal/buildtool <verify|build|package|smoke|mcp-conformance> [--expected-version VERSION] [--archive FILE]")
	}
	mode := buildMode(arguments[0])
	flags := flag.NewFlagSet("buildtool", flag.ContinueOnError)
	expected := flags.String("expected-version", "", "require the root POM version to equal this value")
	archive := flags.String("archive", "", "release archive to verify in smoke mode")
	if err := flags.Parse(arguments[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if mode != modeSmoke && *archive != "" {
		return fmt.Errorf("--archive is valid only in smoke mode")
	}
	paths, err := resolveProjectPaths()
	if err != nil {
		return err
	}
	version, err := readProductVersion(filepath.Join(paths.repository, "pom.xml"))
	if err != nil {
		return err
	}
	if *expected != "" && *expected != version {
		return fmt.Errorf("expected version %q does not match root POM version %q", *expected, version)
	}
	if (mode == modePackage || mode == modeSmoke) && *expected == "" {
		return fmt.Errorf("%s requires --expected-version", mode)
	}
	if mode == modeSmoke {
		if *archive == "" {
			return fmt.Errorf("smoke requires --archive")
		}
		return smokeReleaseArchive(*archive, version)
	}
	fmt.Printf("loomspan Console %s: %s\n", mode, version)
	context := pipelineContext{paths: paths, productVersion: version}
	if err := runPipeline(mode, context, pipelineDependencies{run: realRunner{}.run}); err != nil {
		return err
	}
	if mode == modePackage {
		result, err := packageCurrentTarget(context)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\nCreated %s\n", result.archive, result.sidecar)
	}
	return nil
}
