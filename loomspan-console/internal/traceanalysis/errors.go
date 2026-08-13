package traceanalysis

import (
	"errors"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// InvalidityCategory is the stable internal classification of a rejected
// artifact. It is used for diagnostics and fixture-corpus tests. Outward, every
// content invalidity maps to consolecore.CodeInvalidArtifact with
// rawDownloadAvailable set when the raw download remains usable; the category
// itself is never exposed through adapter DTOs.
type InvalidityCategory string

const (
	CategoryMalformedJSON            InvalidityCategory = "MALFORMED_JSON"
	CategoryInconsistentIdentity     InvalidityCategory = "INCONSISTENT_IDENTITY"
	CategoryNonMonotonicSequence     InvalidityCategory = "NON_MONOTONIC_SEQUENCE"
	CategoryIncompleteChunks         InvalidityCategory = "INCOMPLETE_CHUNKS"
	CategoryInvalidChunks            InvalidityCategory = "INVALID_CHUNKS"
	CategoryMissingCompletion        InvalidityCategory = "MISSING_COMPLETION"
	CategoryNonFinalCompletion       InvalidityCategory = "NON_FINAL_COMPLETION"
	CategoryUnsupportedValue         InvalidityCategory = "UNSUPPORTED_VALUE"
	CategoryContradictoryUsage       InvalidityCategory = "CONTRADICTORY_USAGE"
	CategoryInvalidFrameRelationship InvalidityCategory = "INVALID_FRAME_RELATIONSHIP"
	CategoryInvalidTerminalFailure   InvalidityCategory = "INVALID_TERMINAL_FAILURE"
	CategoryInvalidAttempt           InvalidityCategory = "INVALID_ATTEMPT"
	CategoryInvalidUsage             InvalidityCategory = "INVALID_USAGE"
	CategoryLineTooLarge             InvalidityCategory = "LINE_TOO_LARGE"
	CategoryExcessiveJSONDepth       InvalidityCategory = "EXCESSIVE_JSON_DEPTH"
	CategoryTruncatedInput           InvalidityCategory = "TRUNCATED_INPUT"
)

// invalidityError builds a consolecore.Error for a content invalidity. The raw
// download remains available for every content rejection, so the caller can
// still fetch the original bytes. scopeID is the trace ID used as the domain
// scope identifier.
func invalidityError(category InvalidityCategory, scopeID string) *consolecore.Error {
	return invalidityErrorWithCause(category, scopeID, nil)
}

func invalidityErrorWithCause(category InvalidityCategory, scopeID string, cause error) *consolecore.Error {
	invalidity := error(invalidityCause{category: category})
	if cause != nil {
		invalidity = errors.Join(invalidity, cause)
	}
	return consolecore.NewError(
		consolecore.CodeInvalidArtifact,
		"The trace artifact could not be validated.",
		scopeID,
		consolecore.Details{RawDownloadAvailable: boolPtr(true)},
		invalidity,
	)
}

// invalidityCause wraps an InvalidityCategory so tests and the processor can
// recover the exact category from a consolecore.Error via errors.As.
type invalidityCause struct {
	category InvalidityCategory
}

func (cause invalidityCause) Error() string {
	return string(cause.category)
}

// categoryOf extracts the InvalidityCategory from a consolecore.Error produced
// by the processor. It returns false for errors that do not carry a category
// (for example storage or cancellation failures).
func categoryOf(err *consolecore.Error) (InvalidityCategory, bool) {
	if err == nil {
		return "", false
	}
	cause := invalidityCause{}
	if !errors.As(err, &cause) {
		return "", false
	}
	return cause.category, true
}
