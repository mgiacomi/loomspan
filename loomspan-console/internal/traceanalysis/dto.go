package traceanalysis

import (
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// TraceContext carries the target scope, artifact handle, trace ID, and session
// ID on every reusable query result. Adapters map from these neutral types and
// may not recalculate authoritative facts.
type TraceContext struct {
	// TargetScopeID is the current target scope that owns the artifact handle.
	TargetScopeID target.ScopeID
	// Handle is the opaque artifact handle the query was issued against.
	Handle artifact.Handle
	// TraceID is the stable trace identifier.
	TraceID string
	// SessionID is the stable session identifier.
	SessionID string
}

// TraceSummary is the top-level neutral summary of one processed trace. It
// carries identity, terminal outcome, root frame references, aggregate usage,
// counts, gaps, and uncertainties. Optional/unknown calculations are
// represented explicitly; zero is a recorded value, not an absence marker.
type TraceSummary struct {
	Context            TraceContext
	Outcome            string
	TerminalFailureID  *string
	ConfiguredLimits   *ConfiguredLimits
	RecordCount        int64
	FrameCount         int
	AttemptCount       int
	RetryCount         int
	ValidationCount    int
	FailureCount       int
	PayloadCount       int
	GapCount           int
	UncertaintyCount   int
	RootFrameIDs       []string
	AttributedUsage    Usage
	TerminalUsage      Usage
	UnattributedUsage  Usage
	UnframedAttributed Usage
	UsageComplete      bool
}

// FrameSummary is one frame's neutral hierarchy, timing, and usage result. It
// mirrors frameResult but adds the TraceContext and exposes optional durations
// through pointers so unknown is distinct from recorded zero.
type FrameSummary struct {
	Context       TraceContext
	FrameID       string
	ParentFrameID *string
	// ChildFrameIDs contains the immediate children in canonical frame-open
	// order so adapters can traverse the hierarchy without recalculating it.
	ChildFrameIDs []string
	FrameType     string
	Route         string
	// OpenedTimestampMillis and ClosedTimestampMillis are the authoritative
	// record timestamps for the frame boundaries. ClosedTimestampMillis is nil
	// for an incomplete frame; adapters must not infer it from duration.
	OpenedTimestampMillis   int64
	ClosedTimestampMillis   *int64
	InclusiveDurationMillis *int64
	SelfDurationMillis      *int64
	DirectUsage             Usage
	DirectUsageComplete     bool
	DescendantUsage         Usage
	DescendantUsageComplete bool
	InclusiveUsage          Usage
	InclusiveUsageComplete  bool
	SkillNames              []string
	Outcomes                []string
	AttemptIDs              []string
	RetrySequenceIDs        []string
	ValidationStatuses      []string
	FailureIDs              []string
}

// RecordSummary is one physical or logical record's neutral descriptor. It
// carries identity, type, frame/route/skill references, the raw byte address,
// and an optional inline payload. Representation reports whether the record is
// the logical envelope (chunked payload root) or a physical framework record.
type RecordSummary struct {
	Context         TraceContext
	Sequence        int64
	Type            string
	FrameID         string
	ParentFrameID   string
	FrameType       string
	Route           string
	ThreadName      string
	TimestampMillis int64
	// Representation is "logical" for chunked-payload envelopes and "physical"
	// for framework records (including individual chunk records).
	Representation string
	// IsChunk reports whether this is a PAYLOAD_CHUNK_APPENDED physical record.
	IsChunk bool
	// IsEnvelope reports whether this record is a chunked-payload envelope.
	IsEnvelope bool
	// PayloadID is the chunk payload identifier when this is an envelope or chunk.
	PayloadID string
	// Raw locates this record's bytes inside the raw NDJSON artifact.
	Raw RawAddress
	// InlinePayload is present only when the caller explicitly requested an
	// inline payload and the payload is at most maxInlinePayloadBytes.
	InlinePayload *InlinePayload
}

// InlinePayload is a small reconstructed logical payload inlined into a record
// result. It is only present when explicitly requested and bounded by
// maxInlinePayloadBytes.
type InlinePayload struct {
	ContentType string
	// Bytes is the inlined payload content. For text/plain and application/json
	// it is the decoded UTF-8 content; for other content types it is the raw
	// reconstructed bytes.
	Bytes []byte
}

// AttemptSummary is one model attempt's neutral result.
type AttemptSummary struct {
	Context         TraceContext
	RetrySequenceID string
	AttemptID       string
	AttemptNumber   int64
	Usage           Usage
	UsageComplete   bool
}

// RetrySummary is one retry sequence's aggregated neutral result.
type RetrySummary struct {
	Context         TraceContext
	RetrySequenceID string
	Usage           Usage
	UsageComplete   bool
}

// ValidationSummary is one advisor validation cross-reference.
type ValidationSummary struct {
	Context         TraceContext
	Status          string
	RetrySequenceID string
	AttemptID       string
	AttemptNumber   int64
}

// FailureSummary is one ERROR_RECORDED failure fact.
type FailureSummary struct {
	Context          TraceContext
	FailureID        string
	Terminal         bool
	Sequence         int64
	TimestampMillis  int64
	RecordType       string
	FrameID          string
	Route            string
	AttemptID        string
	RetrySequenceID  string
	ValidationStatus string
	ExceptionType    string
	ContextSummary   string
	Diagnostics      []DiagnosticDescriptor
}

type FailureDiagnosticRequest struct {
	Handle    artifact.Handle
	FailureID string
	Ordinal   int
}
type FailureDiagnostic struct {
	Context    TraceContext
	FailureID  string
	Descriptor DiagnosticDescriptor
	Text       string
}

// UsageBreakdown is the component-wise usage breakdown for a trace or frame.
type UsageBreakdown struct {
	Context            TraceContext
	Attributed         Usage
	Unattributed       Usage
	UnframedAttributed Usage
	Terminal           Usage
}

// Gap records one structural gap (for example an open frame never closed).
type Gap struct {
	Context   TraceContext
	Kind      string
	FrameID   string
	AttemptID string
}

// Uncertainty records one explicit calculation uncertainty.
type Uncertainty struct {
	Context TraceContext
	Kind    string
	FrameID string
}

// PayloadDescriptor describes one reconstructed logical payload and locates it
// inside the payload store component.
type PayloadDescriptor struct {
	Context     TraceContext
	PayloadID   string
	Sequence    int64
	ContentType string
	ChunkCount  int
	StoreOffset int64
	StoreLength int64
}

// RawRecordDescriptor locates one physical record's bytes inside the raw
// NDJSON artifact. It is the addressable form for raw-record ranges.
type RawRecordDescriptor struct {
	Context        TraceContext
	Sequence       int64
	RecordType     string
	Raw            RawAddress
	Representation string
}

// Page is a finite, continuable page of typed results. Items is always
// non-nil (possibly empty). NextCursor is empty when the page is the final
// one for this query.
type Page[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

// ByteRangeResult is a bounded byte range from a payload, raw record, or raw
// artifact. Encoding reports whether Content is UTF-8 text or base64-encoded
// arbitrary bytes. ActualStart/ActualEnd report the byte offsets actually
// returned (which may differ from the request for UTF-8 boundary alignment).
type ByteRangeResult struct {
	Context     TraceContext
	Source      RangeSource
	ActualStart int64
	ActualEnd   int64
	TotalLength int64
	ContentType string
	Encoding    RangeEncoding
	Content     []byte
	NextCursor  string
	HasMore     bool
}

// RangeSource identifies which artifact component a byte range was read from.
type RangeSource string

const (
	// RangeSourcePayload is a reconstructed logical payload range.
	RangeSourcePayload RangeSource = "PAYLOAD"
	// RangeSourceRawRecord is a raw physical record range inside the raw
	// NDJSON artifact.
	RangeSourceRawRecord RangeSource = "RAW_RECORD"
	// RangeSourceRawArtifact is a raw artifact range spanning one or more
	// physical records.
	RangeSourceRawArtifact RangeSource = "RAW_ARTIFACT"
)

// RangeEncoding reports how a ByteRangeResult's Content is encoded.
type RangeEncoding string

const (
	// RangeEncodingText means Content is valid UTF-8 text.
	RangeEncodingText RangeEncoding = "TEXT"
	// RangeEncodingBase64 means Content is base64-encoded arbitrary bytes
	// that could not be represented as a complete UTF-8 slice.
	RangeEncodingBase64 RangeEncoding = "BASE64"
)

// SearchResult is one literal-text search match inside a record or payload.
type SearchResult struct {
	Context    TraceContext
	Sequence   int64
	RecordType string
	FrameID    string
	// MatchOffset is the byte offset of the match within the searched content.
	MatchOffset int64
	// MatchLength is the byte length of the matched literal.
	MatchLength int
	// SearchedField reports which field the match was found in.
	SearchedField string
}
