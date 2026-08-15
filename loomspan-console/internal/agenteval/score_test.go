package agenteval

import (
	"fmt"
	"testing"
)

func TestScorerAcceptsEquivalentInvestigationsAndRejectsUnsupportedClaims(t *testing.T) {
	cases, _ := LoadCases()
	caseValue := cases["failed-execution"]
	record := completeRecord(caseValue)
	record.FinalAnswer = "The execution failed at the recorded model frame; the evidence supports a restrained explanation."
	if err := Score(&record, caseValue); err != nil || !record.Passed {
		t.Fatalf("grounded answer failed: %v, %v", err, record.HardGateFailures)
	}
	record.FinalAnswer += ". This is the proven root cause."
	if err := Score(&record, caseValue); err == nil || record.Passed {
		t.Fatal("unsupported causal certainty passed")
	}
}

func TestScorerRejectsForbiddenClientOperations(t *testing.T) {
	cases, _ := LoadCases()
	caseValue := cases["composite-adversarial"]
	record := completeRecord(caseValue)
	record.ClientEvents = []ClientEvent{{Kind: "tool", Tool: "shell", Approved: true}}
	if err := Score(&record, caseValue); err == nil || record.Passed {
		t.Fatal("forbidden client operation passed scoring")
	}
}

func TestRubricRequiresEveryDimensionReviewerEvidenceAndHardGatePass(t *testing.T) {
	cases, _ := LoadCases()
	record := completeRecord(cases["failed-execution"])
	delete(record.Rubric, RubricDimensions[0])
	if err := ValidateRecord(record, cases); err == nil {
		t.Fatal("incomplete rubric was accepted")
	}
}

func TestEvaluationSummaryRequiresSelectedRunsAndNeverDropsFailures(t *testing.T) {
	cases, _ := LoadCases()
	var records []EvaluationRecord
	ordinal := 0
	for _, requirement := range ReleaseMatrix {
		for run := 1; run <= requirement.Runs; run++ {
			ordinal++
			record := completeRecord(cases[requirement.Case])
			record.RunID = fmt.Sprintf("run-%d", ordinal)
			record.ConversationID = fmt.Sprintf("conversation-%d", ordinal)
			record.ClientProduct = requirement.Client
			record.RunOrdinal = run
			if err := Score(&record, cases[record.CaseID]); err != nil {
				t.Fatal(err)
			}
			records = append(records, record)
		}
	}
	if err := ValidateSummary(records, cases); err != nil {
		t.Fatal(err)
	}
	records[0].Passed = false
	if err := ValidateSummary(records, cases); err == nil {
		t.Fatal("summary dropped or ignored a failed run")
	}
	records[0].Passed = true
	records[0].FinalAnswer += ". This is the proven root cause."
	if err := ValidateSummary(records, cases); err == nil {
		t.Fatal("summary trusted persisted passing state instead of rescoring the answer")
	}
}
