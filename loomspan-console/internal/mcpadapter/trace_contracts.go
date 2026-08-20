package mcpadapter

import (
	"encoding/json"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
)

const (
	ListTracesToolName        = "LOOMSPAN_list_traces"
	GetTraceToolName          = "LOOMSPAN_get_trace"
	QueryTraceFramesToolName  = "LOOMSPAN_query_trace_frames"
	QueryTraceRecordsToolName = "LOOMSPAN_query_trace_records"
	ReadTraceContentToolName  = "LOOMSPAN_read_trace_content"
	ReadTraceArtifactToolName = "LOOMSPAN_read_trace_artifact"
	maxTraceTokenLength       = 8192
	maxTraceRangeBytes        = traceanalysis.MaxRangeBytes
	defaultTraceRangeBytes    = traceanalysis.DefaultRangeBytes
)

type listTracesInput struct {
	PageSize      int                             `json:"pageSize,omitempty"`
	Continuation  string                          `json:"continuation,omitempty"`
	Sources       []traceinventory.EvidenceSource `json:"sources,omitempty"`
	Outcomes      []string                        `json:"outcomes,omitempty"`
	EntrySkill    string                          `json:"entrySkill,omitempty"`
	SessionID     string                          `json:"sessionId,omitempty"`
	FinalizedFrom *time.Time                      `json:"finalizedFrom,omitempty"`
	FinalizedTo   *time.Time                      `json:"finalizedTo,omitempty"`
	AcquiredFrom  *time.Time                      `json:"acquiredFrom,omitempty"`
	AcquiredTo    *time.Time                      `json:"acquiredTo,omitempty"`
	ImportedFrom  *time.Time                      `json:"importedFrom,omitempty"`
	ImportedTo    *time.Time                      `json:"importedTo,omitempty"`
	Order         traceinventory.Order            `json:"order,omitempty"`
}
type getTraceInput struct {
	TraceID string `json:"traceId"`
}
type queryTraceFramesInput struct {
	TraceID      string                        `json:"traceId"`
	Filter       traceanalysis.FrameFilter     `json:"filter,omitempty"`
	Order        traceanalysis.FrameOrder      `json:"order,omitempty"`
	Projection   traceanalysis.FrameProjection `json:"projection,omitempty"`
	PageSize     int                           `json:"pageSize,omitempty"`
	Continuation string                        `json:"continuation,omitempty"`
}
type queryTraceRecordsInput struct {
	TraceID        string                             `json:"traceId"`
	Filter         traceanalysis.RecordFilter         `json:"filter,omitempty"`
	Representation traceanalysis.RecordRepresentation `json:"representation,omitempty"`
	InlineContent  bool                               `json:"inlineContent,omitempty"`
	PageSize       int                                `json:"pageSize,omitempty"`
	Continuation   string                             `json:"continuation,omitempty"`
}
type traceRangeInput struct {
	TraceID      string `json:"traceId"`
	ContentRef   string `json:"contentRef,omitempty"`
	Start        *int64 `json:"start,omitempty"`
	Continuation string `json:"continuation,omitempty"`
	MaxBytes     int    `json:"maxBytes,omitempty"`
}

type evidenceDTO struct {
	TraceID    string    `json:"traceId"`
	SessionID  string    `json:"sessionId"`
	ObservedAt time.Time `json:"observedAt"`
}
type traceLimitationDTO struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type traceInventoryItemDTO struct {
	TraceID         string                          `json:"traceId"`
	EvidenceSources []traceinventory.EvidenceSource `json:"evidenceSources"`
	SessionID       *string                         `json:"sessionId,omitempty"`
	EntrySkill      *string                         `json:"entrySkill,omitempty"`
	Outcome         *string                         `json:"outcome,omitempty"`
	FinalizedAt     *time.Time                      `json:"finalizedAt,omitempty"`
	AcquiredAt      *time.Time                      `json:"acquiredAt,omitempty"`
	ImportedAt      *time.Time                      `json:"importedAt,omitempty"`
	Ambiguous       bool                            `json:"ambiguous,omitempty"`
}
type listTracesResult struct {
	ObservedAt   time.Time               `json:"observedAt"`
	Items        []traceInventoryItemDTO `json:"items"`
	Complete     bool                    `json:"complete"`
	Limitations  []traceLimitationDTO    `json:"limitations,omitempty"`
	HasMore      bool                    `json:"hasMore"`
	Continuation string                  `json:"continuation,omitempty"`
}

type usageDTO struct {
	PromptUnits     int64 `json:"promptUnits"`
	CompletionUnits int64 `json:"completionUnits"`
	TotalUnits      int64 `json:"totalUnits"`
}
type traceSummaryDTO struct {
	Outcome                 string                          `json:"outcome"`
	TerminalFailureID       *string                         `json:"terminalFailureId,omitempty"`
	ConfiguredLimits        *traceanalysis.ConfiguredLimits `json:"configuredLimits,omitempty"`
	RecordCount             int64                           `json:"recordCount"`
	FrameCount              int                             `json:"frameCount"`
	AttemptCount            int                             `json:"attemptCount"`
	RetryCount              int                             `json:"retryCount"`
	ValidationCount         int                             `json:"validationCount"`
	FailureCount            int                             `json:"failureCount"`
	PayloadCount            int                             `json:"payloadCount"`
	GapCount                int                             `json:"gapCount"`
	UncertaintyCount        int                             `json:"uncertaintyCount"`
	RootFrameIDs            []string                        `json:"rootFrameIds"`
	AttributedUsage         usageDTO                        `json:"attributedUsage"`
	TerminalUsage           usageDTO                        `json:"terminalUsage"`
	UnattributedUsage       usageDTO                        `json:"unattributedUsage"`
	UnframedAttributedUsage usageDTO                        `json:"unframedAttributedUsage"`
	UsageComplete           bool                            `json:"usageComplete"`
}
type getTraceResult struct {
	Evidence evidenceDTO     `json:"evidence"`
	Summary  traceSummaryDTO `json:"summary"`
}

type frameDTO struct {
	FrameID                 string    `json:"frameId"`
	ParentFrameID           *string   `json:"parentFrameId,omitempty"`
	ChildFrameIDs           []string  `json:"childFrameIds"`
	FrameType               string    `json:"frameType"`
	Route                   string    `json:"route,omitempty"`
	OpenedTimestampMillis   int64     `json:"openedTimestampMillis"`
	ClosedTimestampMillis   *int64    `json:"closedTimestampMillis,omitempty"`
	InclusiveDurationMillis *int64    `json:"inclusiveDurationMillis,omitempty"`
	SelfDurationMillis      *int64    `json:"selfDurationMillis,omitempty"`
	DirectUsage             *usageDTO `json:"directUsage,omitempty"`
	DirectUsageComplete     bool      `json:"directUsageComplete,omitempty"`
	DescendantUsage         *usageDTO `json:"descendantUsage,omitempty"`
	DescendantUsageComplete bool      `json:"descendantUsageComplete,omitempty"`
	InclusiveUsage          *usageDTO `json:"inclusiveUsage,omitempty"`
	InclusiveUsageComplete  bool      `json:"inclusiveUsageComplete,omitempty"`
	SkillNames              []string  `json:"skillNames,omitempty"`
	Outcomes                []string  `json:"outcomes"`
	AttemptIDs              []string  `json:"attemptIds,omitempty"`
	RetrySequenceIDs        []string  `json:"retrySequenceIds,omitempty"`
	ValidationStatuses      []string  `json:"validationStatuses,omitempty"`
	FailureIDs              []string  `json:"failureIds,omitempty"`
	GapKinds                []string  `json:"gapKinds,omitempty"`
	UncertaintyKinds        []string  `json:"uncertaintyKinds,omitempty"`
	DirectAttemptCount      int       `json:"directAttemptCount"`
	DirectRetryCount        int       `json:"directRetryCount"`
	DirectValidationCount   int       `json:"directValidationCount"`
	DirectFailureCount      int       `json:"directFailureCount"`
	GapCount                int       `json:"gapCount"`
	UncertaintyCount        int       `json:"uncertaintyCount"`
	detailed                bool
}
type queryFramesResult struct {
	Evidence     evidenceDTO `json:"evidence"`
	Projection   string      `json:"projection"`
	Items        []frameDTO  `json:"items"`
	HasMore      bool        `json:"hasMore"`
	Continuation string      `json:"continuation,omitempty"`
}

type diagnosticDTO struct {
	Ordinal           int    `json:"ordinal"`
	Kind              string `json:"kind"`
	ContentType       string `json:"contentType"`
	Truncated         bool   `json:"truncated"`
	CaptureLimitBytes int    `json:"captureLimitBytes"`
	DecodedBytes      int    `json:"decodedBytes"`
	ContentRef        string `json:"contentRef"`
}
type attemptDTO struct {
	RetrySequenceID       string   `json:"retrySequenceId"`
	AttemptID             string   `json:"attemptId"`
	AttemptNumber         int64    `json:"attemptNumber"`
	AttemptReason         string   `json:"attemptReason"`
	ProviderAttemptNumber int64    `json:"providerAttemptNumber"`
	Outcome               string   `json:"outcome"`
	FailureClassification string   `json:"failureClassification,omitempty"`
	FailureCategory       string   `json:"failureCategory,omitempty"`
	RetryDecision         string   `json:"retryDecision,omitempty"`
	RetryDelayMillis      int64    `json:"retryDelayMillis"`
	RetryDelaySource      string   `json:"retryDelaySource,omitempty"`
	HTTPStatus            int64    `json:"httpStatus,omitempty"`
	ProviderErrorType     string   `json:"providerErrorType,omitempty"`
	ProviderErrorCode     string   `json:"providerErrorCode,omitempty"`
	ContentRef            string   `json:"contentRef,omitempty"`
	Usage                 usageDTO `json:"usage"`
	UsageComplete         bool     `json:"usageComplete"`
}
type retryDTO struct {
	RetrySequenceID string   `json:"retrySequenceId"`
	Usage           usageDTO `json:"usage"`
	UsageComplete   bool     `json:"usageComplete"`
}
type validationDTO struct {
	Status          string `json:"status"`
	RetrySequenceID string `json:"retrySequenceId"`
	AttemptID       string `json:"attemptId"`
	AttemptNumber   int64  `json:"attemptNumber"`
}
type failureDTO struct {
	FailureID        string          `json:"failureId"`
	Terminal         bool            `json:"terminal"`
	Sequence         int64           `json:"sequence"`
	TimestampMillis  int64           `json:"timestampMillis"`
	RecordType       string          `json:"recordType"`
	FrameID          string          `json:"frameId,omitempty"`
	Route            string          `json:"route,omitempty"`
	AttemptID        string          `json:"attemptId,omitempty"`
	RetrySequenceID  string          `json:"retrySequenceId,omitempty"`
	ValidationStatus string          `json:"validationStatus,omitempty"`
	ExceptionType    string          `json:"exceptionType,omitempty"`
	ContextSummary   string          `json:"contextSummary,omitempty"`
	Diagnostics      []diagnosticDTO `json:"diagnostics"`
}
type searchMatchDTO struct {
	Sequence      int64  `json:"sequence"`
	RecordType    string `json:"recordType"`
	FrameID       string `json:"frameId,omitempty"`
	MatchOffset   int64  `json:"matchOffset"`
	MatchLength   int    `json:"matchLength"`
	SearchedField string `json:"searchedField"`
	ContentID     string `json:"contentId,omitempty"`
}
type searchContentDescriptorDTO struct {
	ContentID  string `json:"contentId"`
	ContentRef string `json:"contentRef"`
}
type recordFactsDTO struct {
	Plan          *planLandmarkDTO `json:"plan,omitempty"`
	Attempts      []attemptDTO     `json:"attempts"`
	Retries       []retryDTO       `json:"retries"`
	Validations   []validationDTO  `json:"validations"`
	Failures      []failureDTO     `json:"failures"`
	SearchMatches []searchMatchDTO `json:"searchMatches"`
}
type planLandmarkDTO struct {
	PlanID           string `json:"planId"`
	Sequence         int64  `json:"sequence"`
	TraceRootFrameID string `json:"traceRootFrameId"`
	MissionFrameID   string `json:"missionFrameId"`
	PlanningFrameID  string `json:"planningFrameId"`
	AttemptID        string `json:"attemptId,omitempty"`
	RetrySequenceID  string `json:"retrySequenceId,omitempty"`
}
type contentDescriptorDTO struct {
	Role              string `json:"role"`
	ContentType       string `json:"contentType"`
	Encoding          string `json:"encoding"`
	RetainedBytes     int64  `json:"retainedBytes"`
	Available         bool   `json:"available"`
	Complete          bool   `json:"complete"`
	InlineEligibility bool   `json:"inlineEligibility"`
	InlineOmission    string `json:"inlineOmission,omitempty"`
	ContentRef        string `json:"contentRef,omitempty"`
	InlineContent     string `json:"inlineContent,omitempty"`
}
type recordDTO struct {
	Sequence        int64                    `json:"sequence"`
	Type            string                   `json:"type"`
	FailureID       string                   `json:"failureId,omitempty"`
	FrameID         string                   `json:"frameId,omitempty"`
	ParentFrameID   string                   `json:"parentFrameId,omitempty"`
	FrameType       string                   `json:"frameType,omitempty"`
	Route           string                   `json:"route,omitempty"`
	ThreadName      string                   `json:"threadName"`
	TimestampMillis int64                    `json:"timestampMillis"`
	Representation  string                   `json:"representation"`
	IsChunk         bool                     `json:"isChunk"`
	IsEnvelope      bool                     `json:"isEnvelope"`
	Raw             traceanalysis.RawAddress `json:"raw"`
	Content         *contentDescriptorDTO    `json:"content,omitempty"`
	Facts           recordFactsDTO           `json:"facts"`
}
type queryRecordsResult struct {
	Evidence           evidenceDTO                   `json:"evidence"`
	Items              []recordDTO                   `json:"items,omitempty"`
	Matches            []searchMatchDTO              `json:"matches,omitempty"`
	ContentDescriptors *[]searchContentDescriptorDTO `json:"contentDescriptors,omitempty"`
	Search             *searchCoverageDTO            `json:"search,omitempty"`
	HasMore            bool                          `json:"hasMore"`
	Continuation       string                        `json:"continuation,omitempty"`
}

func (value queryRecordsResult) MarshalJSON() ([]byte, error) {
	type alias queryRecordsResult
	body, err := json.Marshal(alias(value))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if value.Search == nil {
		fields["items"] = nonNil(value.Items)
	} else {
		if value.Matches == nil {
			value.Matches = []searchMatchDTO{}
		}
		fields["matches"] = value.Matches
	}
	return json.Marshal(fields)
}

type searchCoverageDTO struct {
	Query                   string               `json:"query"`
	CaseSensitive           bool                 `json:"caseSensitive"`
	Representation          string               `json:"representation"`
	SearchedFields          []string             `json:"searchedFields"`
	SemanticContentCoverage string               `json:"semanticContentCoverage"`
	WorkComplete            bool                 `json:"workComplete"`
	Limitations             []traceLimitationDTO `json:"limitations"`
}
type rangeResult struct {
	Evidence     evidenceDTO `json:"evidence"`
	ActualStart  int64       `json:"actualStart"`
	ActualEnd    int64       `json:"actualEnd"`
	TotalLength  int64       `json:"totalLength"`
	ContentType  string      `json:"contentType"`
	Encoding     string      `json:"encoding"`
	Content      string      `json:"content"`
	HasMore      bool        `json:"hasMore"`
	Continuation string      `json:"continuation,omitempty"`
}

func traceInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return schema
}

func enumProperty(schema *jsonschema.Schema, name string, values ...string) {
	if property := schema.Properties[name]; property != nil {
		property.Enum = make([]any, len(values))
		for i, v := range values {
			property.Enum[i] = v
		}
	}
}
func boundedInteger(schema *jsonschema.Schema, name string, minimum, maximum float64) {
	if p := schema.Properties[name]; p != nil {
		p.Type = "integer"
		p.Types = nil
		p.Minimum = &minimum
		p.Maximum = &maximum
	}
}
func boundedString(schema *jsonschema.Schema, name string, max int) {
	if p := schema.Properties[name]; p != nil {
		min := 1
		p.MinLength = &min
		p.MaxLength = &max
	}
}
func nonblankBoundedString(schema *jsonschema.Schema, name string, max int) {
	boundedString(schema, name, max)
	if p := schema.Properties[name]; p != nil {
		p.Pattern = `.*\S.*`
	}
}
func exactlyOne(schema *jsonschema.Schema, first, second string) {
	schema.OneOf = []*jsonschema.Schema{{Required: []string{first}, Not: &jsonschema.Schema{Required: []string{second}}}, {Required: []string{second}, Not: &jsonschema.Schema{Required: []string{first}}}}
}
func usageValue(v traceanalysis.Usage) usageDTO {
	return usageDTO{v.PromptUnits, v.CompletionUnits, v.TotalUnits}
}
func rangeContent(value traceanalysis.ByteRangeResult) string {
	return string(value.Content)
}
