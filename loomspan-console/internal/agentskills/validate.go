package agentskills

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

const (
	RuntimeDebuggingSkillName = "loomspan"
	maxDescriptionBytes       = 1024
	maxInstructionBytes       = 80 * 1024
	maxReferenceBytes         = 80 * 1024
	maxRecommendedTokens      = 5000
)

var RuntimeDebuggingFiles = []string{
	"SKILL.md",
	"references/common-failure-patterns.md",
	"references/debugging-playbooks.md",
	"references/evidence-and-confidence.md",
	"references/mcp-tool-guide.md",
	"references/runtime-model.md",
}

var (
	referenceLink = regexp.MustCompile(`\]\((references/[^)#?]+\.md)\)`)
	endpoint      = regexp.MustCompile(`(?i)\bhttps?://[^\s)>]+`)
	mcpKey        = regexp.MustCompile(`\blsmcp_[A-Za-z0-9_-]+`)
	authHeader    = regexp.MustCompile(`(?i)authorization\s*:\s*bearer`)
	traceData     = regexp.MustCompile(`(?i)(\.ndjson\b|"recordType"\s*:|"traceId"\s*:)`)
)

type manifest struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	License       string `yaml:"license"`
	Compatibility string `yaml:"compatibility"`
}

// ValidateRuntimeDebugging validates the one reviewed Agent Skill package.
// It intentionally does not provide a generic plugin or client extension API.
func ValidateRuntimeDebugging(directory string) error {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Errorf("resolve skill directory: %w", err)
	}
	if filepath.Base(abs) != RuntimeDebuggingSkillName {
		return fmt.Errorf("skill parent directory must be %q", RuntimeDebuggingSkillName)
	}
	if err := rejectLinkedRoot(abs); err != nil {
		return err
	}
	contents, err := loadExactPackage(abs)
	if err != nil {
		return err
	}
	frontmatter, body, err := splitFrontmatter(contents["SKILL.md"])
	if err != nil {
		return err
	}
	if err := validateManifest(frontmatter); err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maxInstructionBytes || estimatedTokens(body) > maxRecommendedTokens {
		return fmt.Errorf("SKILL.md instruction body exceeds the %d-token budget or is empty", maxRecommendedTokens)
	}
	if err := validateReferences(body, contents); err != nil {
		return err
	}
	for name, content := range contents {
		limit := maxReferenceBytes
		if name == "SKILL.md" {
			limit = maxInstructionBytes + 8*1024
		}
		if len(content) > limit || !utf8.Valid(content) {
			return fmt.Errorf("%s is oversized or not valid UTF-8", name)
		}
		if endpoint.Match(content) || mcpKey.Match(content) || authHeader.Match(content) || traceData.Match(content) {
			return fmt.Errorf("%s contains an endpoint, credential/header value, or generated trace data", name)
		}
	}
	return nil
}

func rejectLinkedRoot(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect skill directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || isReparsePoint(info) {
		return fmt.Errorf("skill directory must be a real directory, not a link")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return fmt.Errorf("resolve skill directory links: %w", err)
	}
	left, right := filepath.Clean(directory), filepath.Clean(resolved)
	if runtime.GOOS == "windows" {
		left, right = strings.ToLower(left), strings.ToLower(right)
	}
	if left != right {
		return fmt.Errorf("skill directory must not traverse a symlink or reparse point")
	}
	return nil
}

func loadExactPackage(directory string) (map[string][]byte, error) {
	expected := make(map[string]bool, len(RuntimeDebuggingFiles)+1)
	expected["references"] = true
	for _, name := range RuntimeDebuggingFiles {
		expected[name] = true
	}
	var found []string
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == directory {
			return nil
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !expected[relative] {
			return fmt.Errorf("unexpected skill package path %q", relative)
		}
		info, err := os.Lstat(name)
		if err != nil {
			return err
		}
		if relative == "references" {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || isReparsePoint(info) {
				return fmt.Errorf("references must be a real directory")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || entry.Type()&fs.ModeSymlink != 0 || isReparsePoint(info) {
			return fmt.Errorf("%s must be a regular non-linked file", relative)
		}
		found = append(found, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	if strings.Join(found, "\n") != strings.Join(RuntimeDebuggingFiles, "\n") {
		return nil, fmt.Errorf("skill package files are incomplete: found %v", found)
	}
	result := make(map[string][]byte, len(found))
	for _, name := range found {
		content, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		result[name] = content
	}
	return result, nil
}

func splitFrontmatter(content []byte) ([]byte, []byte, error) {
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, nil, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	end := bytes.Index(normalized[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, nil, fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	end += 4
	return normalized[4:end], bytes.TrimSpace(normalized[end+5:]), nil
}

func validateManifest(raw []byte) error {
	var fields map[string]any
	if err := yaml.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	allowed := map[string]bool{"name": true, "description": true, "license": true, "compatibility": true}
	for name := range fields {
		if !allowed[name] {
			return fmt.Errorf("unsupported SKILL.md frontmatter field %q", name)
		}
	}
	var value manifest
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("parse typed SKILL.md frontmatter: %w", err)
	}
	if value.Name != RuntimeDebuggingSkillName {
		return fmt.Errorf("skill name must be %q", RuntimeDebuggingSkillName)
	}
	if strings.TrimSpace(value.Description) == "" || len(value.Description) > maxDescriptionBytes {
		return fmt.Errorf("skill description must be nonempty and at most %d bytes", maxDescriptionBytes)
	}
	if value.License != "MPL-2.0" || strings.TrimSpace(value.Compatibility) == "" {
		return fmt.Errorf("skill license or compatibility is invalid")
	}
	return nil
}

func validateReferences(body []byte, contents map[string][]byte) error {
	counts := make(map[string]int)
	for _, match := range referenceLink.FindAllSubmatch(body, -1) {
		name := string(match[1])
		if strings.Count(name, "/") != 1 || filepath.ToSlash(filepath.Clean(name)) != name {
			return fmt.Errorf("reference link %q must be one level and relative", name)
		}
		counts[name]++
	}
	for _, name := range RuntimeDebuggingFiles[1:] {
		if counts[name] != 1 {
			return fmt.Errorf("SKILL.md must link reference %q exactly once", name)
		}
		if len(contents[name]) == 0 {
			return fmt.Errorf("reference %q is empty", name)
		}
	}
	for name := range counts {
		if _, ok := contents[name]; !ok {
			return fmt.Errorf("broken reference link %q", name)
		}
	}
	return nil
}

func estimatedTokens(body []byte) int {
	// A conservative dependency-free release guard; official skills-ref remains
	// independent standards evidence in CI.
	return (len(body) + 3) / 4
}
