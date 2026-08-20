package agenteval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const CaseSchemaVersion = 1

var RequiredCapabilities = []string{
	"loomspan.runtime-status.v1",
	"loomspan.skill-inspection.v1",
	"loomspan.active-execution-inspection.v1",
	"loomspan.recent-activity-inspection.v1",
	"loomspan.trace-inspection.v1",
}

const OptionalRawCapability = "loomspan.raw-artifact-inspection.v1"

type Case struct {
	SchemaVersion             int      `json:"schemaVersion"`
	ID                        string   `json:"id"`
	WorkflowIDs               []string `json:"workflowIds,omitempty"`
	RequirementIDs            []string `json:"requirementIds,omitempty"`
	DeveloperPrompt           string   `json:"developerPrompt"`
	FixtureSources            []string `json:"fixtureSources"`
	Capabilities              []string `json:"capabilities"`
	MCPAvailable              *bool    `json:"mcpAvailable,omitempty"`
	SkillAvailable            *bool    `json:"skillAvailable,omitempty"`
	ExpectedFacts             []string `json:"expectedFacts"`
	ForbiddenClaims           []string `json:"forbiddenClaims"`
	ForbiddenActions          []string `json:"forbiddenActions"`
	RequiredIdentifierClasses []string `json:"requiredIdentifierClasses"`
	LimitationFacts           []string `json:"limitationFacts"`
}

func RepositoryRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve agent evaluation source")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(source), "..", "..", ".."))
}

func LoadCases() (map[string]Case, error) {
	repository, err := RepositoryRoot()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(repository, "loomspan-console", "agent-evals", "cases")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make(map[string]Case)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("unexpected evaluation case path %q", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var value Case
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("parse case %s: %w", entry.Name(), err)
		}
		if err := ValidateCase(value, repository); err != nil {
			return nil, fmt.Errorf("case %s: %w", entry.Name(), err)
		}
		if strings.TrimSuffix(entry.Name(), ".json") != value.ID {
			return nil, fmt.Errorf("case filename %q does not match ID %q", entry.Name(), value.ID)
		}
		if _, exists := result[value.ID]; exists {
			return nil, fmt.Errorf("duplicate case ID %q", value.ID)
		}
		result[value.ID] = value
	}
	return result, nil
}

func ValidateCase(value Case, repository string) error {
	if value.SchemaVersion != CaseSchemaVersion || value.ID == "" || strings.TrimSpace(value.DeveloperPrompt) == "" {
		return fmt.Errorf("schema version, ID, and developer prompt are required")
	}
	approvedWorkflows := map[string]bool{
		"WF-FAILED-EXECUTION": true, "WF-SLOW-EXECUTION": true,
		"WF-EXPENSIVE-EXECUTION": true, "WF-UNFAMILIAR-SKILL-PATH": true,
	}
	if len(value.WorkflowIDs)+len(value.RequirementIDs) == 0 {
		return fmt.Errorf("an approved workflow or requirement ID is required")
	}
	for _, id := range value.WorkflowIDs {
		if !approvedWorkflows[id] {
			return fmt.Errorf("unknown workflow ID %q", id)
		}
	}
	for _, id := range value.RequirementIDs {
		if !strings.HasPrefix(id, "PR19-") && !strings.HasPrefix(id, "PR28-") && !strings.HasPrefix(id, "PR30-") && !strings.HasPrefix(id, "PR31-") && !strings.HasPrefix(id, "WF-") {
			return fmt.Errorf("unknown requirement ID %q", id)
		}
	}
	for _, collection := range [][]string{value.FixtureSources, value.ExpectedFacts, value.ForbiddenClaims,
		value.ForbiddenActions, value.RequiredIdentifierClasses, value.LimitationFacts} {
		if len(collection) == 0 {
			return fmt.Errorf("all case oracle collections must be nonempty")
		}
		if duplicates(collection) {
			return fmt.Errorf("case oracle collections must not contain duplicates")
		}
	}
	for _, source := range value.FixtureSources {
		clean := filepath.Clean(filepath.FromSlash(source))
		if filepath.IsAbs(clean) || strings.HasPrefix(filepath.ToSlash(clean), "../") {
			return fmt.Errorf("unsafe fixture source %q", source)
		}
		if _, err := os.Stat(filepath.Join(repository, clean)); err != nil {
			return fmt.Errorf("fixture source %q is unavailable: %w", source, err)
		}
	}
	validCapabilities := append(append([]string{}, RequiredCapabilities...), OptionalRawCapability)
	allowed := make(map[string]bool, len(validCapabilities))
	for _, capability := range validCapabilities {
		allowed[capability] = true
	}
	if duplicates(value.Capabilities) {
		return fmt.Errorf("capabilities contain duplicates")
	}
	for _, capability := range value.Capabilities {
		if !allowed[capability] {
			return fmt.Errorf("unknown capability %q", capability)
		}
	}
	return nil
}

func SortedCaseIDs(cases map[string]Case) []string {
	result := make([]string, 0, len(cases))
	for id := range cases {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func duplicates(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}
