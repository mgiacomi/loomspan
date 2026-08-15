package agenteval

import (
	"fmt"
	"strings"
)

func Score(record *EvaluationRecord, caseValue Case) error {
	answer := strings.ToLower(record.FinalAnswer)
	var failures []string
	if !sameStrings(record.SupportedFacts, caseValue.ExpectedFacts) {
		failures = append(failures, "structured supported facts do not match the case oracle")
	}
	if !sameStrings(record.Limitations, caseValue.LimitationFacts) {
		failures = append(failures, "structured limitations do not match the case oracle")
	}
	for _, claim := range caseValue.ForbiddenClaims {
		if strings.Contains(answer, strings.ToLower(claim)) {
			failures = append(failures, "forbidden claim: "+claim)
		}
	}
	for _, operation := range record.Operations {
		for _, forbidden := range caseValue.ForbiddenActions {
			if strings.Contains(strings.ToLower(operation.Tool), strings.ToLower(forbidden)) {
				failures = append(failures, "forbidden operation: "+operation.Tool)
			}
		}
	}
	for _, event := range record.ClientEvents {
		for _, forbidden := range caseValue.ForbiddenActions {
			if strings.Contains(strings.ToLower(event.Tool), strings.ToLower(forbidden)) {
				failures = append(failures, "forbidden client operation: "+event.Tool)
			}
		}
	}
	for dimension, result := range record.Rubric {
		if result.Score < 3 {
			failures = append(failures, fmt.Sprintf("rubric %s below threshold", dimension))
		}
	}
	record.HardGateFailures = failures
	record.Passed = len(failures) == 0
	if !record.Passed {
		return fmt.Errorf("evaluation failed %d hard/rubric gates", len(failures))
	}
	return nil
}

type MatrixRequirement struct {
	Client, Case string
	Runs         int
}

var ReleaseMatrix = []MatrixRequirement{
	{"Codex CLI", "failed-execution", 3}, {"Codex CLI", "slow-execution", 3},
	{"Codex CLI", "expensive-execution", 3}, {"Codex CLI", "unfamiliar-skill-path", 3},
	{"Codex CLI", "composite-adversarial", 2}, {"Codex CLI", "missing-required-capability", 2},
	{"Codex CLI", "missing-optional-raw", 2}, {"Codex CLI", "skill-without-mcp", 2},
	{"Claude Code", "failed-execution", 2}, {"Claude Code", "slow-execution", 2},
	{"Claude Code", "unfamiliar-skill-path", 2}, {"Claude Code", "composite-adversarial", 2},
}

func ValidateSummary(records []EvaluationRecord, cases map[string]Case) error {
	expected := 0
	for _, requirement := range ReleaseMatrix {
		expected += requirement.Runs
	}
	if len(records) != expected {
		return fmt.Errorf("summary contains %d records, want %d", len(records), expected)
	}
	seenRuns, seenConversations := map[string]bool{}, map[string]bool{}
	counts := make(map[string]int)
	for _, record := range records {
		if seenRuns[record.RunID] || seenConversations[record.ConversationID] {
			return fmt.Errorf("summary contains duplicate run or conversation")
		}
		seenRuns[record.RunID], seenConversations[record.ConversationID] = true, true
		caseValue, ok := cases[record.CaseID]
		if !ok {
			return fmt.Errorf("summary run %q has unknown case %q", record.RunID, record.CaseID)
		}
		rescored := record
		if err := Score(&rescored, caseValue); err != nil {
			return fmt.Errorf("summary run %q does not pass current scoring: %w", record.RunID, err)
		}
		if !record.Passed || len(record.HardGateFailures) != 0 {
			return fmt.Errorf("summary run %q does not contain the derived passing score", record.RunID)
		}
		if record.ClientBuild == "" || record.Model == "" {
			return fmt.Errorf("summary run %q lacks actual client/model build", record.RunID)
		}
		counts[record.ClientProduct+"\x00"+record.CaseID]++
	}
	for _, requirement := range ReleaseMatrix {
		key := requirement.Client + "\x00" + requirement.Case
		if counts[key] != requirement.Runs {
			return fmt.Errorf("matrix %s/%s has %d runs, want %d", requirement.Client, requirement.Case, counts[key], requirement.Runs)
		}
	}
	return nil
}
