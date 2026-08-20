package traceanalysis

import "github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"

// attemptBuild is the working state for one model attempt.
type attemptBuild struct {
	retrySequenceID       string
	attemptID             string
	attemptNumber         int64
	attemptReason         string
	providerAttemptNumber int64
	usage                 Usage
	usageComplete         bool
	hasResponse           bool
	hasFailure            bool
	failureClassification string
	failureCategory       string
	retryDecision         string
	retryDelayMillis      int64
	retryDelaySource      string
	httpStatus            int64
	providerErrorType     string
	providerErrorCode     string
	payloadID             string
	lifecycle             []TraceRecordType
	ownerSequence         int64
}

// attemptGraph tracks attempt/retry membership using only explicit attemptId
// and retrySequenceId. It validates positive consistent attempt numbers and
// lifecycle ordering for consumed model request/response facts. It also collects
// advisor validation links keyed by their recorded attempt identity.
type attemptGraph struct {
	attempts        map[string]*attemptBuild // keyed by attemptId
	order           []string                 // attemptId in first-seen order
	lastByRetry     map[string]*attemptBuild
	validationLinks []validationLink
}

// newAttemptGraph creates an empty attempt graph.
func newAttemptGraph() *attemptGraph {
	return &attemptGraph{attempts: map[string]*attemptBuild{}, lastByRetry: map[string]*attemptBuild{}}
}

// onAdvisorRecord processes an ADVISOR_REQUEST_MUTATION_RECORDED or
// ADVISOR_RESPONSE_MUTATION_RECORDED record, capturing its validation link.
func (g *attemptGraph) onAdvisorRecord(rec *Record) *consolecore.Error {
	status := rec.metadataStringOrEmpty("status")
	retryID := rec.metadataStringOrEmpty("retrySequenceId")
	attemptID := rec.metadataStringOrEmpty("attemptId")
	number, present, valid := rec.metadataIntStrict("attemptNumber")
	attempt, exists := g.attempts[attemptID]
	if status == "" || retryID == "" || attemptID == "" || !present || !valid || number <= 0 ||
		!exists || attempt.retrySequenceID != retryID || attempt.attemptNumber != number {
		return invalidityError(CategoryInvalidAttempt, rec.TraceID)
	}
	g.validationLinks = append(g.validationLinks, validationLink{
		Status:          status,
		RetrySequenceID: retryID,
		AttemptID:       attemptID,
		AttemptNumber:   number,
		sequence:        rec.Sequence,
	})
	return nil
}

// onModelRecord processes a MODEL_REQUEST_SENT, MODEL_RESPONSE_RECEIVED, or
// MODEL_ATTEMPT_FAILED record. It validates attempt identity consistency and
// lifecycle ordering.
func (g *attemptGraph) onModelRecord(rec *Record) *consolecore.Error {
	attemptID := rec.metadataStringOrEmpty("attemptId")
	retryID := rec.metadataStringOrEmpty("retrySequenceId")
	numberStr, numberPresent, numberValid := rec.metadataIntStrict("attemptNumber")
	reason := rec.metadataStringOrEmpty("attemptReason")
	providerNumber, providerPresent, providerValid := rec.metadataIntStrict("providerAttemptNumber")
	if attemptID == "" || retryID == "" || !numberPresent || !numberValid || numberStr <= 0 ||
		!providerPresent || !providerValid || providerNumber <= 0 ||
		(reason != "INITIAL" && reason != "SEMANTIC_RETRY" && reason != "PROVIDER_RETRY") {
		return invalidityError(CategoryInvalidAttempt, rec.TraceID)
	}
	a, exists := g.attempts[attemptID]
	if !exists {
		previous := g.lastByRetry[retryID]
		if previous == nil {
			if numberStr != 1 || providerNumber != 1 || reason != "INITIAL" {
				return invalidityError(CategoryInvalidAttempt, rec.TraceID)
			}
		} else {
			if numberStr != previous.attemptNumber+1 || (!previous.hasResponse && !previous.hasFailure) {
				return invalidityError(CategoryInvalidAttempt, rec.TraceID)
			}
			switch reason {
			case "PROVIDER_RETRY":
				if !previous.hasFailure || previous.retryDecision != "RETRY" || providerNumber != previous.providerAttemptNumber+1 {
					return invalidityError(CategoryInvalidAttempt, rec.TraceID)
				}
			case "SEMANTIC_RETRY":
				if !previous.hasResponse || providerNumber != 1 {
					return invalidityError(CategoryInvalidAttempt, rec.TraceID)
				}
			default:
				return invalidityError(CategoryInvalidAttempt, rec.TraceID)
			}
		}
		a = &attemptBuild{
			retrySequenceID:       retryID,
			attemptID:             attemptID,
			attemptNumber:         numberStr,
			attemptReason:         reason,
			providerAttemptNumber: providerNumber,
		}
		g.attempts[attemptID] = a
		g.order = append(g.order, attemptID)
		g.lastByRetry[retryID] = a
	} else {
		// Identity consistency: retrySequenceId and attemptNumber must not change.
		if a.retrySequenceID != retryID || a.attemptNumber != numberStr || a.attemptReason != reason || a.providerAttemptNumber != providerNumber {
			return invalidityError(CategoryInvalidAttempt, rec.TraceID)
		}
	}
	// Lifecycle ordering: SENT -> RESPONSE_RECEIVED|ATTEMPT_FAILED, no repeats.
	if !lifecycleAccepts(a.lifecycle, rec.Type) {
		return invalidityError(CategoryInvalidAttempt, rec.TraceID)
	}
	a.lifecycle = append(a.lifecycle, rec.Type)
	a.ownerSequence = rec.Sequence

	if rec.Type == RecordModelResponseReceived {
		a.hasResponse = true
		usage, complete, ok := extractResponseUsage(rec)
		if !ok {
			return invalidityError(CategoryInvalidUsage, rec.TraceID)
		}
		a.usage = usage
		a.usageComplete = complete
	}
	if rec.Type == RecordModelAttemptFailed {
		a.hasFailure = true
		a.failureClassification = rec.metadataStringOrEmpty("failureClassification")
		a.failureCategory = rec.metadataStringOrEmpty("failureCategory")
		a.retryDecision = rec.metadataStringOrEmpty("retryDecision")
		a.retryDelaySource = rec.metadataStringOrEmpty("retryDelaySource")
		delay, present, valid := rec.metadataIntStrict("retryDelayMillis")
		if !present || !valid || delay < 0 ||
			(a.failureClassification != "TRANSIENT" && a.failureClassification != "PERMANENT" && a.failureClassification != "UNKNOWN") ||
			(a.retryDecision != "RETRY" && a.retryDecision != "DO_NOT_RETRY" && a.retryDecision != "ATTEMPTS_EXHAUSTED") ||
			(a.retryDelaySource != "NONE" && a.retryDelaySource != "BACKOFF" && a.retryDelaySource != "RETRY_AFTER") ||
			(a.retryDecision == "RETRY" && a.failureClassification != "TRANSIENT") ||
			(a.retryDecision == "RETRY" && a.retryDelaySource == "NONE") ||
			(a.retryDecision != "RETRY" && (delay != 0 || a.retryDelaySource != "NONE")) {
			return invalidityError(CategoryInvalidAttempt, rec.TraceID)
		}
		a.retryDelayMillis = delay
		a.httpStatus, _, _ = rec.metadataIntStrict("httpStatus")
		a.providerErrorType = rec.metadataStringOrEmpty("providerErrorType")
		a.providerErrorCode = rec.metadataStringOrEmpty("providerErrorCode")
		a.payloadID = rec.PayloadID
	}
	return nil
}

// lifecycleAccepts reports whether recType can follow the existing lifecycle.
func lifecycleAccepts(lifecycle []TraceRecordType, recType TraceRecordType) bool {
	switch recType {
	case RecordModelRequestSent:
		return len(lifecycle) == 0
	case RecordModelResponseReceived, RecordModelAttemptFailed:
		return len(lifecycle) == 1 && lifecycle[0] == RecordModelRequestSent
	}
	return false
}

// extractResponseUsage extracts and validates the usage object from a
// MODEL_RESPONSE_RECEIVED record. It returns the usage, whether it is complete
// (precision != UNAVAILABLE and present), and whether the usage was valid.
func extractResponseUsage(rec *Record) (Usage, bool, bool) {
	return extractUsage(rec, "usage")
}

// extractUsage extracts a usage object from a metadata key. It returns the
// usage, whether it is complete (precision present and != UNAVAILABLE), and
// whether the usage was valid. Absent usage is zero with complete=false.
func extractUsage(rec *Record, key string) (Usage, bool, bool) {
	m, err := rec.metadataObject()
	if err != nil {
		return Usage{}, false, false
	}
	raw, ok := m[key]
	if !ok || isNullRaw(raw) {
		return Usage{}, false, true
	}
	var u usagePayload
	if err := jsonUnmarshal(raw, &u); err != nil {
		return Usage{}, false, false
	}
	if u.PromptUnits == nil || u.CompletionUnits == nil || u.TotalUnits == nil || u.Precision == nil {
		return Usage{}, false, false
	}
	if _, known := knownPrecision(*u.Precision); !known {
		return Usage{}, false, false
	}
	complete := *u.Precision != string(PrecisionUnavailable)
	usage := Usage{PromptUnits: *u.PromptUnits, CompletionUnits: *u.CompletionUnits, TotalUnits: *u.TotalUnits}
	if !validateUsageComponents(usage, complete) {
		return Usage{}, false, false
	}
	return usage, complete, true
}
