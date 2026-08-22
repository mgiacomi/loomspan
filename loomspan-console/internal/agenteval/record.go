package agenteval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpadapter"
)

const RecordSchemaVersion = 1

var RubricDimensions = []string{
	"factual-grounding", "useful-explanation", "stable-identifier-citation",
	"evidence-calculation-context-inference", "appropriate-uncertainty",
	"direct-limitations", "capability-error-distinction", "adversarial-resistance",
}

type Operation struct {
	Tool               string   `json:"tool"`
	ArgumentHash       string   `json:"argumentHash"`
	ResultHash         string   `json:"resultHash"`
	FixtureIDs         []string `json:"fixtureIds,omitempty"`
	EvidenceReferences []string `json:"evidenceReferences,omitempty"`
}

type ClientEvent struct {
	Kind       string `json:"kind"`
	Tool       string `json:"tool,omitempty"`
	Approved   bool   `json:"approved"`
	ResultHash string `json:"resultHash,omitempty"`
}

type RubricResult struct {
	Score int    `json:"score"`
	Note  string `json:"note"`
}

type EvaluationRecord struct {
	SchemaVersion       int                     `json:"schemaVersion"`
	RunID               string                  `json:"runId"`
	ConversationID      string                  `json:"conversationId"`
	CaseID              string                  `json:"caseId"`
	WorkflowIDs         []string                `json:"workflowIds,omitempty"`
	RecordedAt          time.Time               `json:"recordedAt"`
	OS                  string                  `json:"os"`
	ClientProduct       string                  `json:"clientProduct"`
	ClientBuild         string                  `json:"clientBuild"`
	Model               string                  `json:"model"`
	ConsoleVersion      string                  `json:"consoleVersion"`
	ConsoleCommit       string                  `json:"consoleCommit"`
	RunOrdinal          int                     `json:"runOrdinal"`
	MCPProtocol         string                  `json:"mcpProtocol"`
	Capabilities        []string                `json:"capabilities"`
	Operations          []Operation             `json:"operations"`
	Identifiers         map[string][]string     `json:"identifiers"`
	SupportedFacts      []string                `json:"supportedFacts"`
	Limitations         []string                `json:"limitations"`
	FinalAnswer         string                  `json:"finalAnswer"`
	EventStreamKind     string                  `json:"eventStreamKind"`
	EventStreamComplete bool                    `json:"eventStreamComplete"`
	ClientEvents        []ClientEvent           `json:"clientEvents"`
	Rubric              map[string]RubricResult `json:"rubric"`
	HardGateFailures    []string                `json:"hardGateFailures"`
	Passed              bool                    `json:"passed"`
}

type ClientTranscript struct {
	SchemaVersion       int                     `json:"schemaVersion"`
	RunID               string                  `json:"runId"`
	ConversationID      string                  `json:"conversationId"`
	ClientProduct       string                  `json:"clientProduct"`
	ClientBuild         string                  `json:"clientBuild"`
	Model               string                  `json:"model"`
	RunOrdinal          int                     `json:"runOrdinal"`
	MCPProtocol         string                  `json:"mcpProtocol"`
	Capabilities        []string                `json:"capabilities"`
	Operations          []Operation             `json:"operations"`
	Identifiers         map[string][]string     `json:"identifiers"`
	SupportedFacts      []string                `json:"supportedFacts"`
	Limitations         []string                `json:"limitations"`
	EventStreamKind     string                  `json:"eventStreamKind"`
	EventStreamComplete bool                    `json:"eventStreamComplete"`
	ClientEvents        []ClientEvent           `json:"clientEvents"`
	Rubric              map[string]RubricResult `json:"rubric"`
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`lsmcp_[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`(?i)authorization\s*:\s*bearer`),
	regexp.MustCompile(`(?i)target[_ -]?(key|credential)\s*[:=]`),
	regexp.MustCompile(`(?m)([A-Za-z]:\\+(?:Users|Temp)\\+|/(?:home|Users|tmp)/)`),
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidateRecord(record EvaluationRecord, cases map[string]Case) error {
	if record.SchemaVersion != RecordSchemaVersion || record.RunID == "" || record.ConversationID == "" || record.CaseID == "" {
		return fmt.Errorf("record schema, run, conversation, and case IDs are required")
	}
	caseValue, ok := cases[record.CaseID]
	if !ok {
		return fmt.Errorf("unknown case %q", record.CaseID)
	}
	if record.RecordedAt.IsZero() || record.RecordedAt.Location() != time.UTC {
		return fmt.Errorf("recordedAt must be UTC")
	}
	for name, value := range map[string]string{"os": record.OS, "clientProduct": record.ClientProduct, "clientBuild": record.ClientBuild,
		"model": record.Model, "consoleVersion": record.ConsoleVersion, "consoleCommit": record.ConsoleCommit,
		"mcpProtocol": record.MCPProtocol, "eventStreamKind": record.EventStreamKind} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if record.RunOrdinal < 1 || strings.TrimSpace(record.FinalAnswer) == "" || len(record.FinalAnswer) > 64*1024 {
		return fmt.Errorf("run ordinal and final answer are required")
	}
	if !sameStrings(record.WorkflowIDs, caseValue.WorkflowIDs) {
		return fmt.Errorf("record workflow IDs must match the evaluation case")
	}
	if duplicates(record.Capabilities) {
		return fmt.Errorf("record capabilities contain duplicates")
	}
	allowedCapabilities := make(map[string]bool)
	for _, capability := range append(append([]string{}, RequiredCapabilities...), OptionalRawCapability) {
		allowedCapabilities[capability] = true
	}
	for _, capability := range record.Capabilities {
		if !allowedCapabilities[capability] {
			return fmt.Errorf("fabricated capability %q", capability)
		}
	}
	allowedTools := map[string]bool{
		mcpadapter.RuntimeToolName: true, mcpadapter.ListSkillsToolName: true, mcpadapter.GetSkillToolName: true,
		mcpadapter.ListExecutionsToolName: true, mcpadapter.GetExecutionToolName: true,
		mcpadapter.GetExecutionActivityToolName: true, mcpadapter.ListTracesToolName: true,
		mcpadapter.GetTraceToolName: true, mcpadapter.QueryTraceFramesToolName: true,
		mcpadapter.QueryTraceRecordsToolName: true, mcpadapter.ReadTraceContentToolName: true,
		mcpadapter.ReadTraceArtifactToolName: true,
	}
	for _, operation := range record.Operations {
		if !allowedTools[operation.Tool] || !sha256Pattern.MatchString(operation.ArgumentHash) || !sha256Pattern.MatchString(operation.ResultHash) {
			return fmt.Errorf("operation metadata must use a reviewed tool and SHA-256 hashes")
		}
	}
	mcpExpected := caseValue.MCPAvailable == nil || *caseValue.MCPAvailable
	if record.CaseID == "unsupported-protocol" {
		mcpExpected = false
	}
	if mcpExpected && (len(record.Operations) == 0 || record.Operations[0].Tool != mcpadapter.RuntimeToolName) {
		return fmt.Errorf("MCP evaluation must bootstrap with LOOMSPAN_get_runtime before dependent work")
	}
	if !mcpExpected && len(record.Operations) != 0 {
		return fmt.Errorf("MCP-unavailable evaluation must not record Loomspan operations")
	}
	if len(record.Identifiers) != len(caseValue.RequiredIdentifierClasses) {
		return fmt.Errorf("record must contain exactly the case's required identifier classes")
	}
	for _, identifierClass := range caseValue.RequiredIdentifierClasses {
		values, ok := record.Identifiers[identifierClass]
		if !ok || len(values) == 0 || duplicates(values) {
			return fmt.Errorf("identifier class %q requires unique stable values", identifierClass)
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\t") {
				return fmt.Errorf("identifier class %q contains an invalid value", identifierClass)
			}
		}
	}
	if !sameStrings(record.SupportedFacts, caseValue.ExpectedFacts) || !sameStrings(record.Limitations, caseValue.LimitationFacts) {
		return fmt.Errorf("record structured facts and limitations must match the case oracle")
	}
	if record.EventStreamKind == "headless" && !record.EventStreamComplete {
		return fmt.Errorf("headless tool-safety claims require a complete client event stream")
	}
	for _, event := range record.ClientEvents {
		if strings.TrimSpace(event.Kind) == "" || (event.ResultHash != "" && !sha256Pattern.MatchString(event.ResultHash)) {
			return fmt.Errorf("client event kind and optional result hash are invalid")
		}
		if !event.Approved {
			return fmt.Errorf("client event contains unapproved operation %q", event.Tool)
		}
	}
	if err := validateRubric(record.Rubric); err != nil {
		return err
	}
	serialized, err := json.Marshal(record)
	if err != nil {
		return err
	}
	for _, pattern := range secretPatterns {
		if pattern.Match(serialized) {
			return fmt.Errorf("record contains a credential, header, or machine-specific path")
		}
	}
	return nil
}

func validateRubric(rubric map[string]RubricResult) error {
	if len(rubric) != len(RubricDimensions) {
		return fmt.Errorf("rubric must contain every reviewed dimension")
	}
	for _, dimension := range RubricDimensions {
		result, ok := rubric[dimension]
		if !ok || result.Score < 0 || result.Score > 4 || strings.TrimSpace(result.Note) == "" {
			return fmt.Errorf("rubric dimension %q requires a 0-4 score and reviewer note", dimension)
		}
	}
	return nil
}

func CanonicalJSON(record EvaluationRecord) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func ValidateRecordFile(filename string, cases map[string]Case) (EvaluationRecord, error) {
	content, err := osReadFile(filename)
	if err != nil {
		return EvaluationRecord{}, err
	}
	var record EvaluationRecord
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return EvaluationRecord{}, err
	}
	return record, ValidateRecord(record, cases)
}

func ImportRecord(session Session, transcriptFile, answerFile string, cases map[string]Case) (EvaluationRecord, error) {
	transcriptBytes, err := os.ReadFile(filepath.Clean(transcriptFile))
	if err != nil {
		return EvaluationRecord{}, err
	}
	var transcript ClientTranscript
	decoder := json.NewDecoder(bytes.NewReader(transcriptBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&transcript); err != nil {
		return EvaluationRecord{}, fmt.Errorf("parse client event stream: %w", err)
	}
	if transcript.SchemaVersion != 1 {
		return EvaluationRecord{}, fmt.Errorf("unsupported client event schema version")
	}
	answer, err := os.ReadFile(filepath.Clean(answerFile))
	if err != nil {
		return EvaluationRecord{}, err
	}
	caseValue, ok := cases[session.CaseID]
	if !ok {
		return EvaluationRecord{}, fmt.Errorf("unknown session case %q", session.CaseID)
	}
	record := EvaluationRecord{
		SchemaVersion: RecordSchemaVersion, RunID: transcript.RunID, ConversationID: transcript.ConversationID,
		CaseID: session.CaseID, WorkflowIDs: caseValue.WorkflowIDs, RecordedAt: time.Now().UTC(), OS: currentOS(),
		ClientProduct: transcript.ClientProduct, ClientBuild: transcript.ClientBuild, Model: transcript.Model,
		ConsoleVersion: session.ConsoleVersion, ConsoleCommit: session.ConsoleCommit,
		RunOrdinal: transcript.RunOrdinal, MCPProtocol: transcript.MCPProtocol, Capabilities: transcript.Capabilities,
		Operations: transcript.Operations, Identifiers: transcript.Identifiers, FinalAnswer: strings.TrimSpace(string(answer)), EventStreamKind: transcript.EventStreamKind,
		SupportedFacts: transcript.SupportedFacts, Limitations: transcript.Limitations,
		EventStreamComplete: transcript.EventStreamComplete, ClientEvents: transcript.ClientEvents, Rubric: transcript.Rubric,
		HardGateFailures: []string{},
	}
	if session.Key != "" && (bytes.Contains(transcriptBytes, []byte(session.Key)) || bytes.Contains(answer, []byte(session.Key))) {
		return EvaluationRecord{}, fmt.Errorf("client evidence contains the live MCP key")
	}
	if err := ValidateRecord(record, cases); err != nil {
		return EvaluationRecord{}, err
	}
	return record, nil
}

var osReadFile = func(name string) ([]byte, error) { return os.ReadFile(filepath.Clean(name)) }

func currentOS() string { return runtime.GOOS + "/" + runtime.GOARCH }

func sortedStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) || duplicates(left) || duplicates(right) {
		return false
	}
	a, b := sortedStrings(left), sortedStrings(right)
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
