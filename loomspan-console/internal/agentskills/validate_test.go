package agentskills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalRuntimeDebuggingSkillIsValidAndExact(t *testing.T) {
	root := canonicalSkill(t)
	if err := ValidateRuntimeDebugging(root); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDebuggingSkillDoesNotTeachRemovedMCPWorkflow(t *testing.T) {
	root := canonicalSkill(t)
	stale := []string{
		"claim to evidence: target scope",
		"artifact handle, payload reference",
		"expired evidence",
		"changed scope",
	}
	for _, relative := range RuntimeDebuggingFiles {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(content))
		for _, phrase := range stale {
			if strings.Contains(lower, phrase) {
				t.Fatalf("%s still teaches removed MCP workflow %q", relative, phrase)
			}
		}
	}
}

func TestRuntimeDebuggingSkillValidationRejectsUnsafeAndNonPortableVariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{"extra file", func(t *testing.T, root string) { write(t, filepath.Join(root, "extra.md"), "x") }, "unexpected"},
		{"missing file", func(t *testing.T, root string) { os.Remove(filepath.Join(root, "references", "runtime-model.md")) }, "incomplete"},
		{"unsupported frontmatter", replaceSkill("license: MPL-2.0", "allowed-tools: []\nlicense: MPL-2.0"), "unsupported"},
		{"wrong name", replaceSkill("name: loomspan-runtime-debugging", "name: another-skill"), "name"},
		{"wrong version", replaceSkill("skill-version: \"1.0.0\"", "skill-version: \"2.0.0\""), "version"},
		{"non-string metadata", replaceSkill("skill-version: \"1.0.0\"", "skill-version: 1"), "string"},
		{"broken reference", replaceSkill("references/runtime-model.md", "references/missing.md"), "reference"},
		{"endpoint", func(t *testing.T, root string) { appendSkill(t, root, "\nUse https://example.invalid/mcp\n") }, "endpoint"},
		{"access key", func(t *testing.T, root string) {
			appendSkill(t, root, "\nlsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n")
		}, "credential"},
		{"authorization header", func(t *testing.T, root string) { appendSkill(t, root, "\nAuthorization: Bearer value\n") }, "header"},
		{"generated trace", func(t *testing.T, root string) { appendSkill(t, root, "\nfixture.ndjson\n") }, "trace"},
		{"scripts directory", func(t *testing.T, root string) { write(t, filepath.Join(root, "scripts", "run.ps1"), "exit 0") }, "unexpected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := copyCanonical(t)
			test.mutate(t, root)
			err := ValidateRuntimeDebugging(root)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}

	if runtime.GOOS != "windows" {
		t.Run("symlink file", func(t *testing.T) {
			root := copyCanonical(t)
			name := filepath.Join(root, "references", "runtime-model.md")
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "SKILL.md"), name); err != nil {
				t.Fatal(err)
			}
			if err := ValidateRuntimeDebugging(root); err == nil {
				t.Fatal("symlink was accepted")
			}
		})
	}
}

func canonicalSkill(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "agent-skills", RuntimeDebuggingSkillName))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyCanonical(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), RuntimeDebuggingSkillName)
	for _, relative := range RuntimeDebuggingFiles {
		content, err := os.ReadFile(filepath.Join(canonicalSkill(t), filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(root, filepath.FromSlash(relative)), string(content))
	}
	return root
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceSkill(old, replacement string) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		name := filepath.Join(root, "SKILL.md")
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		updated := strings.Replace(string(content), old, replacement, 1)
		if updated == string(content) {
			t.Fatalf("mutation source %q not found", old)
		}
		write(t, name, updated)
	}
}

func appendSkill(t *testing.T, root, suffix string) {
	t.Helper()
	name := filepath.Join(root, "SKILL.md")
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	write(t, name, string(content)+suffix)
}
