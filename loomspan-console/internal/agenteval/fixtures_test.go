package agenteval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationCasesAreVersionedUniqueAndWorkflowLinked(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"failed-execution", "slow-execution", "expensive-execution", "unfamiliar-skill-path",
		"composite-adversarial", "ambiguous-trace", "missing-required-capability", "missing-optional-raw", "skill-without-mcp", "mcp-without-skill"} {
		if _, ok := cases[required]; !ok {
			t.Errorf("missing case %q", required)
		}
	}
}

func TestEvaluationCasesResolveAuthoritativeFixtureFacts(t *testing.T) {
	repository, err := RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][]string{
		"loomspan-console-fixtures/expected/runtime-terminal-failure.json":       {"trace-runtime-terminal-failure", "failure-terminal", "FAILED"},
		"loomspan-console-fixtures/expected/nested-frame-usage.json":             {"trace-nested-frame-usage", "attempt-framed"},
		"loomspan-console-fixtures/expected/unattributed-usage.json":             {"trace-unattributed-usage", "unattributed"},
		"loomspan-console-fixtures/expected/repeated-skill-invocations.json":     {"trace-repeated-skill-invocations", "skill-1", "skill-2"},
		"loomspan-console-fixtures/traces/current-plan-semantic-evidence.ndjson": {"framework-primary-plan", "framework-nested-plan", "attempt-accepted", "retry-primary", "INC-2401", "TOOL_CALL_COMPLETED", "STRUCTURED_OUTPUT_RECORDED"},
		"loomspan-console/internal/mcpadapter/testdata/activity.json":            {"observedAt", "canonicalSequence", "beginningUnavailable", "reset"},
	}
	for relative, markers := range checks {
		content, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(content), marker) {
				t.Errorf("fixture %s does not contain authoritative marker %q", relative, marker)
			}
		}
	}
}

func TestEvaluationDegradationCasesHaveDistinctExpectedClassifications(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	required := cases["missing-required-capability"]
	optional := cases["missing-optional-raw"]
	if contains(required.Capabilities, "loomspan.trace-inspection.v1") || !contains(required.Capabilities, OptionalRawCapability) {
		t.Fatal("required-capability case does not isolate trace inspection")
	}
	if !contains(optional.Capabilities, "loomspan.trace-inspection.v1") || contains(optional.Capabilities, OptionalRawCapability) {
		t.Fatal("optional-capability case does not isolate raw inspection")
	}
}

func TestTraceInterfaceEvaluationCasesUseOnlyLLMFacingIdentifiers(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	rejected := map[string]bool{
		"sourceFilter":   true,
		"source":         true,
		"artifactHandle": true,
		"targetScopeId":  true,
		"instanceId":     true,
		"resourceUri":    true,
	}
	for _, value := range cases {
		for _, identifier := range value.RequiredIdentifierClasses {
			if rejected[identifier] {
				t.Errorf("case %q requires rejected identifier class %q", value.ID, identifier)
			}
		}
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
