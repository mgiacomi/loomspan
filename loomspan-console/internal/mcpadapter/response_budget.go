package mcpadapter

import "encoding/json"

const (
	// defaultTraceResultBudget is the committed ceiling for ordinary trace
	// navigation responses. Exact caller-selected range reads are exempt.
	defaultTraceResultBudget = 32 << 10
	defaultRangeResultBudget = 48 << 10
	// pageEnvelopeReserve covers the JSON-RPC result envelope, evidence/header
	// fields, arrays, hasMore, and an opaque continuation.
	pageEnvelopeReserve = 20 << 10
)

type pageAdmission struct {
	budget int
	used   int
}

func newPageAdmission() *pageAdmission {
	return &pageAdmission{budget: defaultTraceResultBudget, used: pageEnvelopeReserve}
}

// Inline record pages account for the actual trace identifier carried in both
// response forms. The ordinary worst-case reserve would otherwise reject one
// legal 8 KiB inline value solely because that value is faithfully repeated in
// the text fallback.
func newInlineRecordPageAdmission(traceID string) *pageAdmission {
	structuredTraceID, _ := json.Marshal(traceID)
	fallbackTraceID, _ := json.Marshal(fallbackField(traceID))
	const inlinePageEnvelopeBase = 4 << 10
	return &pageAdmission{budget: defaultTraceResultBudget, used: inlinePageEnvelopeBase + len(structuredTraceID) + len(fallbackTraceID)}
}

func (a *pageAdmission) admit(structured any, fallbackLine string) bool {
	item, err := json.Marshal(structured)
	if err != nil {
		return false
	}
	line, err := json.Marshal(fallbackLine)
	if err != nil {
		return false
	}
	cost := len(item) + len(line) + 2 // array separator/newline reserve
	if a.used+cost > a.budget {
		return false
	}
	a.used += cost
	return true
}
