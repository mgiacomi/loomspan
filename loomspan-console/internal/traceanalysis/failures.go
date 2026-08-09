package traceanalysis

import (
	"encoding/json"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"strings"
	"unicode/utf8"
)

// failureGraph tracks ERROR_RECORDED failure facts and validates terminal
// failure linkage. Failure identity uses only explicit failureId and
// terminalFailureId.
type failureGraph struct {
	failures map[string]failureResult // failureId -> directly recorded fact
	order    []string                 // failureId in first-seen order
}

// newFailureGraph creates an empty failure graph.
func newFailureGraph() *failureGraph {
	return &failureGraph{failures: map[string]failureResult{}}
}

// onErrorRecord processes an ERROR_RECORDED record. It records the failureId
// and rejects records without the canonical failure identity. The terminal
// linkage check resolves the completion's terminalFailureId against recorded
// failures.
func (g *failureGraph) onErrorRecord(rec *Record) *consolecore.Error {
	failureID := rec.metadataStringOrEmpty("failureId")
	if failureID == "" {
		return invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	fact := failureResult{FailureID: failureID, Sequence: rec.Sequence,
		TimestampMillis: rec.TimestampMillis, RecordType: string(rec.Type), FrameID: rec.FrameID,
		Route: rec.Route, AttemptID: rec.metadataStringOrEmpty("attemptId"),
		RetrySequenceID:  rec.metadataStringOrEmpty("retrySequenceId"),
		ValidationStatus: rec.metadataStringOrEmpty("status"), PayloadID: rec.PayloadID, data: rec.Data}
	if _, dup := g.failures[failureID]; dup {
		return invalidityError(CategoryInvalidTerminalFailure, rec.TraceID)
	}
	g.failures[failureID] = fact
	g.order = append(g.order, failureID)
	return nil
}

type diagnosticValue struct {
	Kind              string  `json:"kind"`
	ContentType       string  `json:"contentType"`
	Text              *string `json:"text"`
	Truncated         *bool   `json:"truncated"`
	CaptureLimitBytes int     `json:"captureLimitBytes"`
}
type failureData struct {
	ExceptionType string            `json:"exceptionType"`
	Message       string            `json:"message"`
	Diagnostics   []diagnosticValue `json:"diagnostics"`
}

func (g *failureGraph) validateDiagnostics(assembler *payloadAssembler, traceID string) *consolecore.Error {
	for _, id := range g.order {
		fact := g.failures[id]
		raw := fact.data
		if fact.PayloadID != "" {
			raw = assembler.logicalPayload(fact.PayloadID)
		}
		if len(raw) == 0 || string(raw) == "null" {
			return invalidityError(CategoryUnsupportedValue, traceID)
		}
		var data failureData
		if err := json.Unmarshal(raw, &data); err != nil {
			return invalidityError(CategoryUnsupportedValue, traceID)
		}
		fact.ExceptionType, fact.ContextSummary = data.ExceptionType, data.Message
		if len(data.Diagnostics) == 0 {
			return invalidityError(CategoryUnsupportedValue, traceID)
		}
		if len(data.Diagnostics) > 16 {
			return invalidityError(CategoryUnsupportedValue, traceID)
		}
		aggregate := 0
		for ordinal, d := range data.Diagnostics {
			if d.Text == nil {
				return invalidityError(CategoryUnsupportedValue, traceID)
			}
			text := *d.Text
			n := len([]byte(text))
			aggregate += n
			if strings.TrimSpace(d.Kind) == "" || len(d.Kind) > 128 || strings.TrimSpace(d.ContentType) == "" || len(d.ContentType) > 256 || d.Truncated == nil || d.CaptureLimitBytes <= 0 || d.CaptureLimitBytes > 1<<20 || n > d.CaptureLimitBytes || n > 1<<20 || !utf8.ValidString(text) || aggregate > 4<<20 {
				return invalidityError(CategoryUnsupportedValue, traceID)
			}
			fact.Diagnostics = append(fact.Diagnostics, DiagnosticDescriptor{Ordinal: ordinal, Kind: d.Kind, ContentType: d.ContentType, Truncated: *d.Truncated, CaptureLimitBytes: d.CaptureLimitBytes, DecodedBytes: n})
		}
		g.failures[id] = fact
	}
	return nil
}

// metadataBoolStrict returns the boolean metadata value and whether it was
// present and non-null.
func (r *Record) metadataBoolStrict(key string) (bool, bool) {
	b, present, err := r.metadataBool(key)
	if err != nil || !present {
		return false, false
	}
	return b, true
}

// validateTerminalLink checks that a failed/aborted terminal outcome has a
// resolvable terminal failure, and a succeeded outcome forbids one. The
// terminalFailureId referenced by TRACE_COMPLETED must match a recorded terminal
// ERROR_RECORDED failure.
func (g *failureGraph) validateTerminalLink(outcome TraceOutcome, terminalFailureID string, traceID string) *consolecore.Error {
	if outcome == OutcomeSucceeded {
		if terminalFailureID != "" {
			return invalidityError(CategoryInvalidTerminalFailure, traceID)
		}
		return nil
	}
	// FAILED or ABORTED requires a resolvable terminal failure.
	if terminalFailureID == "" {
		return invalidityError(CategoryInvalidTerminalFailure, traceID)
	}
	failure, exists := g.failures[terminalFailureID]
	if !exists {
		return invalidityError(CategoryInvalidTerminalFailure, traceID)
	}
	failure.Terminal = true
	g.failures[terminalFailureID] = failure
	return nil
}

// hasTerminalFailure reports whether a terminal failure with the given ID was
// recorded.
func (g *failureGraph) hasTerminalFailure(id string) bool {
	failure, ok := g.failures[id]
	return ok && failure.Terminal
}
