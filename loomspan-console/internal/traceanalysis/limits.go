package traceanalysis

// Fixed operational bounds for the current-release trace parser. These are
// internal correctness/response-framing constants, not configuration. See the
// PR 13 plan's "Fixed Operational Bounds" section: requests outside an accepted
// per-call range return LIMIT_EXCEEDED, and structural violations return
// INVALID_ARTIFACT.
const (
	// maxPhysicalLineBytes is the maximum size of one NDJSON physical line,
	// including the terminating newline handling. A line whose JSON content
	// exceeds this bound is rejected as LINE_TOO_LARGE.
	maxPhysicalLineBytes = 1 << 20 // 1 MiB

	// maxJSONDepth is the maximum nesting depth of JSON objects/arrays in one
	// record. Depth above this bound is rejected as EXCESSIVE_JSON_DEPTH. The
	// bound applies to JSON structure, not to the frame tree.
	maxJSONDepth = 128

	// defaultPageSize is the default page size for finite query results.
	defaultPageSize = 100

	// maxPageSize is the maximum accepted page size for finite query results.
	maxPageSize = 1000

	// maxInlinePayloadBytes is the maximum logical payload size that may be
	// inlined automatically, and only when explicitly requested.
	maxInlinePayloadBytes          = 8 << 10 // 8 KiB
	MaxInlineContentBytes          = 8 << 10
	MaxAggregateInlineContentBytes = 32 << 10
	MaxCompactResponseBytes        = 64 << 10
	MaxDescriptorResponseBytes     = 128 << 10

	// defaultRangeBytes is the default byte range size for payload/raw ranges.
	// DefaultRangeBytes keeps worst-case JSON-escaped TEXT responses, including
	// structured output and deterministic fallback, within the ordinary 32 KiB
	// MCP result ceiling. Explicit callers may still request up to MaxRangeBytes.
	DefaultRangeBytes = 1 << 10 // 1 KiB source bytes
	defaultRangeBytes = DefaultRangeBytes

	// maxRangeBytes is the maximum accepted byte range size per call.
	MaxRangeBytes = 16 << 20 // 16 MiB
	maxRangeBytes = MaxRangeBytes

	// maxLiteralTextBytes is the maximum byte size of a literal search query.
	maxLiteralTextBytes = 1 << 10 // 1 KiB

	// maxLiteralTextRunes is the maximum number of Unicode code points in a
	// literal search query.
	maxLiteralTextRunes = 256

	// maxSearchWorkBytes is the maximum number of fully processed searchable
	// bytes per search call.
	maxSearchWorkBytes = 8 << 20 // 8 MiB

	// maxSearchWorkRecords is the maximum number of records a search call may
	// process.
	maxSearchWorkRecords = 10000
)
