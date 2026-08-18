package mcpadapter

import (
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
)

const (
	ListTracesToolName        = "LOOMSPAN_list_traces"
	GetTraceToolName          = "LOOMSPAN_get_trace"
	QueryTraceFramesToolName  = "LOOMSPAN_query_trace_frames"
	QueryTraceRecordsToolName = "LOOMSPAN_query_trace_records"
	ReadTracePayloadToolName  = "LOOMSPAN_read_trace_payload"
	ReadTraceArtifactToolName = "LOOMSPAN_read_trace_artifact"
	maxTraceTokenLength       = 8192
	maxTraceRangeBytes        = traceanalysis.MaxRangeBytes
	defaultTraceRangeBytes    = traceanalysis.DefaultRangeBytes
)

type listTracesInput struct {
	PageSize     int    `json:"pageSize,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}
type getTraceInput struct {
	TraceID string `json:"traceId"`
}
type queryTraceFramesInput struct {
	TraceID      string                    `json:"traceId"`
	Filter       traceanalysis.FrameFilter `json:"filter,omitempty"`
	Order        traceanalysis.FrameOrder  `json:"order,omitempty"`
	PageSize     int                       `json:"pageSize,omitempty"`
	Continuation string                    `json:"continuation,omitempty"`
}
type queryTraceRecordsInput struct {
	TraceID        string                             `json:"traceId"`
	Filter         traceanalysis.RecordFilter         `json:"filter,omitempty"`
	Representation traceanalysis.RecordRepresentation `json:"representation,omitempty"`
	InlinePayload  bool                               `json:"inlinePayload,omitempty"`
	PageSize       int                                `json:"pageSize,omitempty"`
	Continuation   string                             `json:"continuation,omitempty"`
}
type traceRangeInput struct {
	TraceID      string `json:"traceId"`
	PayloadRef   string `json:"payloadRef,omitempty"`
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
	TraceID     string    `json:"traceId"`
	SessionID   string    `json:"sessionId"`
	EntrySkill  string    `json:"entrySkill"`
	Outcome     string    `json:"outcome"`
	FinalizedAt time.Time `json:"finalizedAt"`
	Ambiguous   bool      `json:"ambiguous,omitempty"`
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
	FrameID                 string   `json:"frameId"`
	ParentFrameID           *string  `json:"parentFrameId,omitempty"`
	ChildFrameIDs           []string `json:"childFrameIds"`
	FrameType               string   `json:"frameType"`
	Route                   string   `json:"route,omitempty"`
	OpenedTimestampMillis   int64    `json:"openedTimestampMillis"`
	ClosedTimestampMillis   *int64   `json:"closedTimestampMillis,omitempty"`
	InclusiveDurationMillis *int64   `json:"inclusiveDurationMillis,omitempty"`
	SelfDurationMillis      *int64   `json:"selfDurationMillis,omitempty"`
	DirectUsage             usageDTO `json:"directUsage"`
	DirectUsageComplete     bool     `json:"directUsageComplete"`
	DescendantUsage         usageDTO `json:"descendantUsage"`
	DescendantUsageComplete bool     `json:"descendantUsageComplete"`
	InclusiveUsage          usageDTO `json:"inclusiveUsage"`
	InclusiveUsageComplete  bool     `json:"inclusiveUsageComplete"`
	SkillNames              []string `json:"skillNames"`
	Outcomes                []string `json:"outcomes"`
	AttemptIDs              []string `json:"attemptIds"`
	RetrySequenceIDs        []string `json:"retrySequenceIds"`
	ValidationStatuses      []string `json:"validationStatuses"`
	FailureIDs              []string `json:"failureIds"`
	GapKinds                []string `json:"gapKinds"`
	UncertaintyKinds        []string `json:"uncertaintyKinds"`
}
type queryFramesResult struct {
	Evidence     evidenceDTO `json:"evidence"`
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
	PayloadRef        string `json:"payloadRef"`
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
	PayloadRef            string   `json:"payloadRef,omitempty"`
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
type payloadDTO struct {
	PayloadRef  string `json:"payloadRef"`
	Sequence    int64  `json:"sequence"`
	ContentType string `json:"contentType"`
	ChunkCount  int    `json:"chunkCount"`
	TotalLength int64  `json:"totalLength"`
}
type searchMatchDTO struct {
	Sequence      int64  `json:"sequence"`
	RecordType    string `json:"recordType"`
	FrameID       string `json:"frameId,omitempty"`
	MatchOffset   int64  `json:"matchOffset"`
	MatchLength   int    `json:"matchLength"`
	SearchedField string `json:"searchedField"`
}
type recordFactsDTO struct {
	Attempts      []attemptDTO     `json:"attempts"`
	Retries       []retryDTO       `json:"retries"`
	Validations   []validationDTO  `json:"validations"`
	Failures      []failureDTO     `json:"failures"`
	Payloads      []payloadDTO     `json:"payloads"`
	SearchMatches []searchMatchDTO `json:"searchMatches"`
}
type inlinePayloadDTO struct {
	ContentType string `json:"contentType"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
}
type recordDTO struct {
	Sequence        int64                    `json:"sequence"`
	Type            string                   `json:"type"`
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
	InlinePayload   *inlinePayloadDTO        `json:"inlinePayload,omitempty"`
	Facts           recordFactsDTO           `json:"facts"`
}
type queryRecordsResult struct {
	Evidence     evidenceDTO `json:"evidence"`
	Items        []recordDTO `json:"items"`
	HasMore      bool        `json:"hasMore"`
	Continuation string      `json:"continuation,omitempty"`
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
