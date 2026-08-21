package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProjectDeclarationsMatchPinnedToolchains(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	nodeVersion, err := os.ReadFile(filepath.Join(paths.module, ".node-version"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(nodeVersion)) != requiredNode {
		t.Fatalf(".node-version = %q", nodeVersion)
	}
	goModule, err := os.ReadFile(filepath.Join(paths.module, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^go 1\.26\.0$`).Match(goModule) ||
		!regexp.MustCompile(`(?m)^toolchain go1\.26\.5$`).Match(goModule) {
		t.Fatalf("go.mod does not declare the pinned toolchain:\n%s", goModule)
	}
}

func TestMCPDependenciesAndSDKBoundaryArePinned(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	goModule := readTestFile(t, filepath.Join(paths.module, "go.mod"))
	if !regexp.MustCompile(`(?m)^\s*github\.com/modelcontextprotocol/go-sdk v1\.7\.0$`).MatchString(goModule) {
		t.Fatal("go.mod does not pin the official MCP Go SDK at v1.7.0")
	}
	manifest := readTestFile(t, filepath.Join(paths.module, "mcp-conformance", "package.json"))
	lock := readTestFile(t, filepath.Join(paths.module, "mcp-conformance", "package-lock.json"))
	const conformanceRevision = "c321dd32035556e6769d3724a8ee97d87c3faaac"
	if !strings.Contains(manifest, conformanceRevision) || !strings.Contains(lock, conformanceRevision) {
		t.Fatal("MCP conformance manifest and lockfile must pin the reviewed revision")
	}
	err = filepath.WalkDir(paths.module, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != paths.module {
			switch entry.Name() {
			case ".git", "build", "dist", "node_modules":
				return fs.SkipDir
			}
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.Contains(filepath.ToSlash(path), "/internal/mcpadapter/") || filepath.Base(path) == "projectdeclarations_test.go" {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(contents), "github.com/modelcontextprotocol/go-sdk") {
			t.Errorf("official MCP SDK import escaped internal/mcpadapter: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOfficialAgentSkillValidatorIsPinnedAndRequired(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	const revision = "69ef37e9424c0a7ea9dd2293b559e43ec8176379"
	manifest := readTestFile(t, filepath.Join(paths.module, "skills-ref-validation", "pyproject.toml"))
	lock := readTestFile(t, filepath.Join(paths.module, "skills-ref-validation", "uv.lock"))
	if !strings.Contains(manifest, revision) || !strings.Contains(lock, revision) || !strings.Contains(lock, "skills-ref") {
		t.Fatal("official skills-ref validator must be locked to the reviewed revision")
	}
	for _, workflow := range []string{"console-ci.yml", "console-release.yml"} {
		contents := readTestFile(t, filepath.Join(paths.repository, ".github", "workflows", workflow))
		for _, required := range []string{"uv==0.11.7", "uv run --frozen --project skills-ref-validation skills-ref validate ./agent-skills/loomspan"} {
			if !strings.Contains(contents, required) {
				t.Errorf("%s does not require pinned Agent Skill validation %q", workflow, required)
			}
		}
	}
}

func TestReleaseLicenseAndRuntimeDocumentExist(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	license, err := os.ReadFile(filepath.Join(paths.repository, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(license)); digest != "fab3dd6bdab226f1c08630b1dd917e11fcb4ec5e1e020e2c16f83a0a13863e85" {
		t.Fatalf("LICENSE is not the canonical MPL 2.0 text: %s", digest)
	}
	readme, err := os.ReadFile(filepath.Join(paths.release, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"--version", "--no-open-browser", "SHA256SUMS", "no JVM", "Target keys"} {
		if !strings.Contains(string(readme), required) {
			t.Errorf("release README does not contain %q", required)
		}
	}
}

func TestReleaseAndAuthoringDocumentationReferenceCanonicalSkillContract(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	documents := map[string]string{
		"Console README":  readTestFile(t, filepath.Join(paths.module, "README.md")),
		"release README":  readTestFile(t, filepath.Join(paths.release, "README.md")),
		"client evidence": readTestFile(t, filepath.Join(paths.module, "docs", "mcp-client-compatibility.md")),
	}
	for name, contents := range documents {
		for _, required := range []string{"skills/loomspan/", "copy", "link", "1.0.1", "MCP"} {
			if !strings.Contains(contents, required) {
				t.Errorf("%s does not contain %q", name, required)
			}
		}
	}
	client := documents["client evidence"]
	for _, required := range []string{"Codex CLI", "Codex desktop", "Claude Code", "Antigravity", "Cursor", "Devin Desktop", "Local Devin CLI", "Hosted Codex", "Not run"} {
		if !strings.Contains(client, required) {
			t.Errorf("client evidence does not retain %q", required)
		}
	}
	authoringREADME := readTestFile(t, filepath.Join(paths.repository, "ai", "skill-authoring", "README.md"))
	authoringTopic := readTestFile(t, filepath.Join(paths.repository, "ai", "skill-authoring", "traces-and-debugging.md"))
	for _, required := range []string{"traces-and-debugging.md", "packaged Agent Skill"} {
		if !strings.Contains(authoringREADME, required) {
			t.Errorf("authoring README does not contain %q", required)
		}
	}
	for _, required := range []string{"loomspan", "loomspan.trace-inspection.v1", "loomspan.raw-artifact-inspection.v1", "defense in depth"} {
		if !strings.Contains(authoringTopic, required) {
			t.Errorf("authoring debugging topic does not contain %q", required)
		}
	}
}

func TestConsoleWorkflowsArePinnedAndLeastPrivilege(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	workflowDirectory := filepath.Join(paths.repository, ".github", "workflows")
	ci := readTestFile(t, filepath.Join(workflowDirectory, "console-ci.yml"))
	releaseWorkflow := readTestFile(t, filepath.Join(workflowDirectory, "console-release.yml"))
	for name, contents := range map[string]string{"console-ci.yml": ci, "console-release.yml": releaseWorkflow} {
		if strings.Contains(contents, "pull_request_target") {
			t.Fatalf("%s executes pull_request_target", name)
		}
		for _, line := range strings.Split(contents, "\n") {
			if strings.Contains(line, "uses:") && !regexp.MustCompile(`uses: [^@]+@[0-9a-f]{40} # v[0-9]+$`).MatchString(strings.TrimSpace(line)) {
				t.Errorf("%s has an unpinned action: %s", name, line)
			}
		}
	}
	for _, required := range []string{"pull_request:", "contents: read", "go-version: 1.26.5", "node-version: 24.18.0", "npm@12.0.2", "-f ../pom.xml help:evaluate", "go run ./internal/buildtool verify", "npm --prefix web run test:e2e", "go run ./internal/buildtool mcp-conformance", "windows-x86_64", "linux-x86_64", "macos-arm64", "macos-x86_64", "macos-15-intel"} {
		if !strings.Contains(ci, required) {
			t.Errorf("CI workflow does not contain %q", required)
		}
	}
	for _, required := range []string{"windows-latest", "ubuntu-latest", "macos-15", "windows-x86_64", "linux-x86_64", "macos-arm64", "workflow_dispatch:", "tags: [\"v*\"]", "SHA256SUMS"} {
		if !strings.Contains(releaseWorkflow, required) {
			t.Errorf("release workflow does not contain %q", required)
		}
	}
	if count := strings.Count(releaseWorkflow, "contents: write"); count != 1 {
		t.Fatalf("release workflow contains %d write grants, want exactly one", count)
	}
}

func readTestFile(t *testing.T, filename string) string {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestPackageManifestUsesExactDirectVersions(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(paths.web, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Private         bool              `json:"private"`
		PackageManager  string            `json:"packageManager"`
		Engines         map[string]string `json:"engines"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.Private || manifest.PackageManager != "npm@"+requiredNPM ||
		manifest.Engines["node"] != requiredNode || manifest.Engines["npm"] != requiredNPM {
		t.Fatalf("package metadata does not match pinned tools: %+v", manifest)
	}
	expected := map[string]string{
		"@axe-core/playwright": "4.12.1",
		"@tailwindcss/vite":    "4.3.3", "react": "19.2.8", "react-aria-components": "1.19.0",
		"react-dom": "19.2.8", "react-router": "8.3.0", "tailwindcss": "4.3.3",
		"@playwright/test": "1.62.0", "@testing-library/dom": "10.4.1",
		"@testing-library/jest-dom": "7.0.0", "@testing-library/react": "16.3.2",
		"@testing-library/user-event": "14.6.1", "@types/react": "19.2.17",
		"@types/react-dom": "19.2.3", "@vitejs/plugin-react": "6.0.4",
		"@vitest/coverage-v8": "4.1.10", "jsdom": "29.1.1", "typescript": "7.0.2",
		"vite": "8.1.5", "vitest": "4.1.10",
	}
	actual := make(map[string]string)
	for name, version := range manifest.Dependencies {
		actual[name] = version
	}
	for name, version := range manifest.DevDependencies {
		actual[name] = version
	}
	if len(actual) != len(expected) {
		t.Fatalf("direct dependency count = %d, want %d", len(actual), len(expected))
	}
	for name, version := range expected {
		if actual[name] != version {
			t.Errorf("%s = %q, want %q", name, actual[name], version)
		}
	}
}
