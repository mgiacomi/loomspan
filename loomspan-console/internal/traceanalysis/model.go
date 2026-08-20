package traceanalysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RawAddress locates one physical record's bytes inside the raw NDJSON
// artifact. Offset is the byte offset of the JSON content (excluding any line
// terminator); Length is the JSON content byte length; TerminatorLength is the
// number of trailing CR/LF bytes. Logical records (chunk envelopes) carry an
// additional reconstructed payload reference.
type RawAddress struct {
	Offset           int64 `json:"offset"`
	Length           int64 `json:"length"`
	TerminatorLength int64 `json:"terminatorLength"`
}

// Record is one parsed physical NDJSON record. It carries the canonical
// consumed fields plus the raw byte address and the opaque unconsumed metadata
// and data payloads (retained as json.RawMessage until written/indexed).
type Record struct {
	// TraceID is the stable trace identifier (non-blank).
	TraceID string
	// SessionID is the stable session identifier (non-blank).
	SessionID string
	// Sequence is the canonical record sequence number.
	Sequence int64
	// TimestampMillis is the record timestamp truncated to milliseconds,
	// matching the Java fixture corpus duration arithmetic.
	TimestampMillis int64
	// Timestamp is the parsed Instant for validation.
	Timestamp time.Time
	// Type is the canonical record type.
	Type TraceRecordType
	// FrameID is the normalized frame identifier, or empty when absent.
	FrameID string
	// ParentFrameID is the normalized parent frame identifier, or empty.
	ParentFrameID string
	// HasParentFrame reports whether ParentFrameID was present (non-null).
	HasParentFrame bool
	// FrameType is the frame type, or empty when absent.
	FrameType TraceFrameType
	// HasFrameType reports whether FrameType was present.
	HasFrameType bool
	// Route is the normalized route, or empty when absent.
	Route string
	// ThreadName is the thread name (blank normalized to "unknown").
	ThreadName string
	// Metadata is the opaque metadata object, retained verbatim.
	Metadata json.RawMessage
	// Data is the opaque data payload, retained verbatim, or nil when absent.
	Data json.RawMessage
	// Raw is the byte address inside the raw artifact.
	Raw RawAddress
	// IsChunk reports whether this is a PAYLOAD_CHUNK_APPENDED physical record.
	IsChunk bool
	// PayloadID is the chunk payload identifier when this is an envelope or chunk.
	PayloadID string
	// IsEnvelope reports whether this record is a chunked-payload envelope
	// (metadata.payloadChunked == true).
	IsEnvelope bool
}

// Usage is the component-wise model usage for one response, attempt, retry,
// frame, or terminal snapshot. Components are summed independently with
// checked arithmetic. Zero is a recorded value, not an absence marker; absence
// is represented separately by completeness flags.
type Usage struct {
	PromptUnits     int64
	CompletionUnits int64
	TotalUnits      int64
}

// ConfiguredLimits is the optional immutable run-start quota snapshot recorded
// by current Spring producers on TRACE_STARTED.
type ConfiguredLimits struct {
	MaxSkillInvocations int64 `json:"maxSkillInvocations"`
	MaxToolInvocations  int64 `json:"maxToolInvocations"`
	MaxLinterRetries    int64 `json:"maxLinterRetries"`
	MaxModelCalls       int64 `json:"maxModelCalls"`
	MaxProviderAttempts int64 `json:"maxProviderAttempts"`
	MaxUsageUnits       int64 `json:"maxUsageUnits"`
}

// usageZero is the recorded zero usage value.
var usageZero = Usage{}

// plus adds two usage values component-wise with checked arithmetic.
func (u Usage) plus(other Usage) (Usage, bool) {
	p, ok := addChecked(u.PromptUnits, other.PromptUnits)
	if !ok {
		return Usage{}, false
	}
	c, ok := addChecked(u.CompletionUnits, other.CompletionUnits)
	if !ok {
		return Usage{}, false
	}
	t, ok := addChecked(u.TotalUnits, other.TotalUnits)
	if !ok {
		return Usage{}, false
	}
	return Usage{PromptUnits: p, CompletionUnits: c, TotalUnits: t}, true
}

// minus subtracts other from u component-wise. The caller validates that no
// component goes negative.
func (u Usage) minus(other Usage) (Usage, bool) {
	p, ok := subChecked(u.PromptUnits, other.PromptUnits)
	if !ok {
		return Usage{}, false
	}
	c, ok := subChecked(u.CompletionUnits, other.CompletionUnits)
	if !ok {
		return Usage{}, false
	}
	t, ok := subChecked(u.TotalUnits, other.TotalUnits)
	if !ok {
		return Usage{}, false
	}
	return Usage{PromptUnits: p, CompletionUnits: c, TotalUnits: t}, true
}

// MarshalJSON renders Usage as the adapter-neutral component map matching the
// Java fixture corpus: {"promptUnits":...,"completionUnits":...,"totalUnits":...}.
func (u Usage) MarshalJSON() ([]byte, error) {
	return json.Marshal(usageJSON{
		PromptUnits:     u.PromptUnits,
		CompletionUnits: u.CompletionUnits,
		TotalUnits:      u.TotalUnits,
	})
}

// usageJSON is the JSON projection of Usage.
type usageJSON struct {
	PromptUnits     int64 `json:"promptUnits"`
	CompletionUnits int64 `json:"completionUnits"`
	TotalUnits      int64 `json:"totalUnits"`
}

// attemptResult is one model attempt's neutral semantic result.
type attemptResult struct {
	RetrySequenceID       string `json:"retrySequenceId"`
	AttemptID             string `json:"attemptId"`
	AttemptNumber         int64  `json:"attemptNumber"`
	AttemptReason         string `json:"attemptReason"`
	ProviderAttemptNumber int64  `json:"providerAttemptNumber"`
	Outcome               string `json:"outcome"`
	FailureClassification string `json:"failureClassification,omitempty"`
	FailureCategory       string `json:"failureCategory,omitempty"`
	RetryDecision         string `json:"retryDecision,omitempty"`
	RetryDelayMillis      int64  `json:"retryDelayMillis"`
	RetryDelaySource      string `json:"retryDelaySource,omitempty"`
	HTTPStatus            int64  `json:"httpStatus,omitempty"`
	ProviderErrorType     string `json:"providerErrorType,omitempty"`
	ProviderErrorCode     string `json:"providerErrorCode,omitempty"`
	PayloadID             string `json:"payloadId,omitempty"`
	Usage                 Usage  `json:"usage"`
	UsageComplete         bool   `json:"usageComplete"`
	ownerSequence         int64
}

// retryResult is one retry sequence's aggregated neutral result.
type retryResult struct {
	RetrySequenceID string `json:"retrySequenceId"`
	Usage           Usage  `json:"usage"`
	UsageComplete   bool   `json:"usageComplete"`
}

// validationLink is one advisor validation cross-reference.
type validationLink struct {
	Status          string `json:"status"`
	RetrySequenceID string `json:"retrySequenceId"`
	AttemptID       string `json:"attemptId"`
	AttemptNumber   int64  `json:"attemptNumber"`
	sequence        int64
}

type persistedFrameResult struct {
	frameResult
	GapKinds         []string `json:"gapKinds,omitempty"`
	UncertaintyKinds []string `json:"uncertaintyKinds,omitempty"`
}

// frameResult is one frame's neutral hierarchy, timing, and usage result.
// Pointer fields distinguish "unknown/unavailable" (nil) from a recorded zero.
type frameResult struct {
	FrameID                 string   `json:"frameId"`
	ParentFrameID           *string  `json:"parentFrameId"`
	ChildFrameIDs           []string `json:"childFrameIds,omitempty"`
	FrameType               string   `json:"frameType"`
	Route                   string   `json:"route,omitempty"`
	OpenedTimestampMillis   int64    `json:"openedTimestampMillis"`
	ClosedTimestampMillis   *int64   `json:"closedTimestampMillis"`
	InclusiveDurationMillis *int64   `json:"inclusiveDurationMillis"`
	SelfDurationMillis      *int64   `json:"selfDurationMillis"`
	DirectUsage             Usage    `json:"directUsage"`
	DirectUsageComplete     bool     `json:"directUsageComplete"`
	DescendantUsage         Usage    `json:"descendantUsage"`
	DescendantUsageComplete bool     `json:"descendantUsageComplete"`
	InclusiveUsage          Usage    `json:"inclusiveUsage"`
	InclusiveUsageComplete  bool     `json:"inclusiveUsageComplete"`
	SkillNames              []string `json:"skillNames,omitempty"`
	Outcome                 *string  `json:"outcome,omitempty"`
	AttemptIDs              []string `json:"attemptIds,omitempty"`
	RetrySequenceIDs        []string `json:"retrySequenceIds,omitempty"`
	ValidationStatuses      []string `json:"validationStatuses,omitempty"`
	FailureIDs              []string `json:"failureIds,omitempty"`
	DirectRetryCount        int      `json:"directRetryCount"`
}

type failureResult struct {
	FailureID        string                 `json:"failureId"`
	Terminal         bool                   `json:"terminal"`
	Sequence         int64                  `json:"sequence"`
	TimestampMillis  int64                  `json:"timestampMillis"`
	RecordType       string                 `json:"recordType"`
	FrameID          string                 `json:"frameId,omitempty"`
	Route            string                 `json:"route,omitempty"`
	AttemptID        string                 `json:"attemptId,omitempty"`
	RetrySequenceID  string                 `json:"retrySequenceId,omitempty"`
	ValidationStatus string                 `json:"validationStatus,omitempty"`
	ExceptionType    string                 `json:"exceptionType,omitempty"`
	ContextSummary   string                 `json:"contextSummary,omitempty"`
	Diagnostics      []DiagnosticDescriptor `json:"diagnostics,omitempty"`
	PayloadID        string                 `json:"payloadId,omitempty"`
	data             json.RawMessage
}

type DiagnosticDescriptor struct {
	Ordinal           int    `json:"ordinal"`
	Kind              string `json:"kind"`
	ContentType       string `json:"contentType"`
	Truncated         bool   `json:"truncated"`
	CaptureLimitBytes int    `json:"captureLimitBytes"`
	DecodedBytes      int    `json:"decodedBytes"`
	ContentRef        string `json:"contentRef,omitempty"`
}

// gapResult records one structural gap (for example an open frame never closed).
type gapResult struct {
	Kind      string `json:"kind"`
	FrameID   string `json:"frameId,omitempty"`
	AttemptID string `json:"attemptId,omitempty"`
}

// uncertaintyResult records one explicit calculation uncertainty.
type uncertaintyResult struct {
	Kind    string `json:"kind"`
	FrameID string `json:"frameId"`
}

// analysisResult is the complete neutral semantic result of one processed
// artifact. Valid results carry every calculated fact; invalid results carry
// only the category. It mirrors the Java fixture-corpus expected schema so the
// Go fixture test can compare directly.
type analysisResult struct {
	Valid                   bool                `json:"valid"`
	TraceID                 string              `json:"traceId,omitempty"`
	SessionID               string              `json:"sessionId,omitempty"`
	Outcome                 string              `json:"outcome,omitempty"`
	TerminalFailureID       *string             `json:"terminalFailureId,omitempty"`
	ConfiguredLimits        *ConfiguredLimits   `json:"configuredLimits,omitempty"`
	AttributedUsage         Usage               `json:"attributedUsage,omitempty"`
	TerminalUsage           Usage               `json:"terminalUsage,omitempty"`
	UnattributedUsage       Usage               `json:"unattributedUsage,omitempty"`
	UsageComplete           bool                `json:"usageComplete,omitempty"`
	Attempts                []attemptResult     `json:"attempts,omitempty"`
	Retries                 []retryResult       `json:"retries,omitempty"`
	ValidationLinks         []validationLink    `json:"validationLinks,omitempty"`
	Frames                  []frameResult       `json:"frames,omitempty"`
	UnframedAttributedUsage Usage               `json:"unframedAttributedUsage,omitempty"`
	Payloads                []payloadDescriptor `json:"payloads,omitempty"`
	Gaps                    []gapResult         `json:"gaps,omitempty"`
	Uncertainties           []uncertaintyResult `json:"uncertainties,omitempty"`
	ErrorCategory           string              `json:"errorCategory,omitempty"`
}

// addChecked adds two int64 values and reports whether the result overflowed.
func addChecked(a, b int64) (int64, bool) {
	r := a + b
	if (a > 0 && b > 0 && r < 0) || (a < 0 && b < 0 && r > 0) {
		return 0, false
	}
	return r, true
}

// subChecked subtracts b from a and reports whether the result overflowed.
func subChecked(a, b int64) (int64, bool) {
	r := a - b
	if (a > 0 && b < 0 && r < 0) || (a < 0 && b > 0 && r > 0) {
		return 0, false
	}
	return r, true
}

// metadataObject unmarshals a record's metadata into a generic map. Records
// always carry a metadata object (the parser substitutes {} when absent).
func (r *Record) metadataObject() (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(r.Metadata, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// metadataString extracts a string metadata field. It returns the value and
// whether the field was present and non-null.
func (r *Record) metadataString(key string) (string, bool, error) {
	m, err := r.metadataObject()
	if err != nil {
		return "", false, err
	}
	raw, ok := m[key]
	if !ok {
		return "", false, nil
	}
	if bytes.Equal(raw, nullBytes) {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false, err
	}
	return s, true, nil
}

// metadataInt extracts an int64 metadata field. It returns the value, whether
// it was present and non-null, and whether it was a valid integer.
func (r *Record) metadataInt(key string) (int64, bool, bool, error) {
	m, err := r.metadataObject()
	if err != nil {
		return 0, false, false, err
	}
	raw, ok := m[key]
	if !ok || bytes.Equal(raw, nullBytes) {
		return 0, false, false, nil
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return 0, true, false, nil
	}
	v, err := n.Int64()
	if err != nil {
		return 0, true, false, nil
	}
	return v, true, true, nil
}

// metadataBool extracts a boolean metadata field.
func (r *Record) metadataBool(key string) (bool, bool, error) {
	m, err := r.metadataObject()
	if err != nil {
		return false, false, err
	}
	raw, ok := m[key]
	if !ok || bytes.Equal(raw, nullBytes) {
		return false, false, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false, err
	}
	return b, true, nil
}

// metadataStringOrEmpty returns the string metadata value or "" when absent or
// unparseable. Strict validation treats unparseable values as invalid.
func (r *Record) metadataStringOrEmpty(key string) string {
	v, present, err := r.metadataString(key)
	if err != nil || !present {
		return ""
	}
	return v
}

// metadataIntStrict returns the int64 metadata value, whether it was present,
// and whether it was a valid integer.
func (r *Record) metadataIntStrict(key string) (int64, bool, bool) {
	v, present, valid, err := r.metadataInt(key)
	if err != nil {
		return 0, present, false
	}
	return v, present, valid
}

var nullBytes = []byte("null")

// boolPtr returns a pointer to b, used to populate Details.RawDownloadAvailable
// without a local variable.
func boolPtr(b bool) *bool {
	return &b
}

// normalizeNullable mirrors Java's normalizeNullable: a null or blank string
// becomes empty (representing absent), otherwise the trimmed value is kept.
func normalizeNullable(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

// formatParseError builds a short message for a JSON parse failure.
func formatParseError(err error) string {
	msg := err.Error()
	if len(msg) > 128 {
		msg = msg[:128]
	}
	return fmt.Sprintf("The trace artifact contains a malformed NDJSON record: %s", msg)
}
