package traceanalysis

// TraceRecordType enumerates the canonical current-release trace record types.
// These mirror com.lokiscale.loomspan.internal.core.TraceRecordType exactly; an
// unknown value is rejected as UNSUPPORTED_VALUE.
type TraceRecordType string

const (
	RecordTraceStarted             TraceRecordType = "TRACE_STARTED"
	RecordTraceCapturePolicy       TraceRecordType = "TRACE_CAPTURE_POLICY_RECORDED"
	RecordFrameOpened              TraceRecordType = "FRAME_OPENED"
	RecordFrameMetadata            TraceRecordType = "FRAME_METADATA_RECORDED"
	RecordPayloadChunkAppended     TraceRecordType = "PAYLOAD_CHUNK_APPENDED"
	RecordModelRequestPrepared     TraceRecordType = "MODEL_REQUEST_PREPARED"
	RecordModelRequestSent         TraceRecordType = "MODEL_REQUEST_SENT"
	RecordAdvisorRequestMutation   TraceRecordType = "ADVISOR_REQUEST_MUTATION_RECORDED"
	RecordModelResponseReceived    TraceRecordType = "MODEL_RESPONSE_RECEIVED"
	RecordModelAttemptFailed       TraceRecordType = "MODEL_ATTEMPT_FAILED"
	RecordAdvisorResponseMutation  TraceRecordType = "ADVISOR_RESPONSE_MUTATION_RECORDED"
	RecordModelThoughtCaptured     TraceRecordType = "MODEL_THOUGHT_CAPTURED"
	RecordPlanCreated              TraceRecordType = "PLAN_CREATED"
	RecordPlanUpdated              TraceRecordType = "PLAN_UPDATED"
	RecordPlanValidationFailed     TraceRecordType = "PLAN_VALIDATION_FAILED"
	RecordPlanRetryRequested       TraceRecordType = "PLAN_RETRY_REQUESTED"
	RecordPlanQualityWarning       TraceRecordType = "PLAN_QUALITY_WARNING"
	RecordToolCallStarted          TraceRecordType = "TOOL_CALL_STARTED"
	RecordToolCallCompleted        TraceRecordType = "TOOL_CALL_COMPLETED"
	RecordToolCallFailed           TraceRecordType = "TOOL_CALL_FAILED"
	RecordEvidenceRecorded         TraceRecordType = "EVIDENCE_RECORDED"
	RecordEvidenceValidationFailed TraceRecordType = "EVIDENCE_VALIDATION_FAILED"
	RecordEvidenceValidationPassed TraceRecordType = "EVIDENCE_VALIDATION_PASSED"
	RecordLinterRecorded           TraceRecordType = "LINTER_RECORDED"
	RecordStructuredOutputRecorded TraceRecordType = "STRUCTURED_OUTPUT_RECORDED"
	RecordStepStarted              TraceRecordType = "STEP_STARTED"
	RecordStepActionProposed       TraceRecordType = "STEP_ACTION_PROPOSED"
	RecordStepActionValidated      TraceRecordType = "STEP_ACTION_VALIDATED"
	RecordStepActionRejected       TraceRecordType = "STEP_ACTION_REJECTED"
	RecordStepCompleted            TraceRecordType = "STEP_COMPLETED"
	RecordErrorRecorded            TraceRecordType = "ERROR_RECORDED"
	RecordFrameClosed              TraceRecordType = "FRAME_CLOSED"
	RecordTraceCompleted           TraceRecordType = "TRACE_COMPLETED"
)

// knownRecordType reports whether value is one of the current-release record
// types.
func knownRecordType(value string) (TraceRecordType, bool) {
	rt := TraceRecordType(value)
	switch rt {
	case RecordTraceStarted, RecordTraceCapturePolicy, RecordFrameOpened, RecordFrameMetadata,
		RecordPayloadChunkAppended, RecordModelRequestPrepared, RecordModelRequestSent,
		RecordAdvisorRequestMutation, RecordModelResponseReceived, RecordModelAttemptFailed, RecordAdvisorResponseMutation,
		RecordModelThoughtCaptured, RecordPlanCreated, RecordPlanUpdated, RecordPlanValidationFailed,
		RecordPlanRetryRequested, RecordPlanQualityWarning, RecordToolCallStarted,
		RecordToolCallCompleted, RecordToolCallFailed, RecordEvidenceRecorded, RecordEvidenceValidationFailed,
		RecordEvidenceValidationPassed, RecordLinterRecorded, RecordStructuredOutputRecorded, RecordStepStarted,
		RecordStepActionProposed, RecordStepActionValidated, RecordStepActionRejected, RecordStepCompleted,
		RecordErrorRecorded, RecordFrameClosed, RecordTraceCompleted:
		return rt, true
	}
	return "", false
}

// TraceFrameType enumerates the canonical current-release frame types. These
// mirror com.lokiscale.loomspan.internal.core.TraceFrameType exactly.
type TraceFrameType string

const (
	FrameRootMission    TraceFrameType = "ROOT_MISSION"
	FrameSkillExecution TraceFrameType = "SKILL_EXECUTION"
	FrameModelCall      TraceFrameType = "MODEL_CALL"
	FrameToolInvocation TraceFrameType = "TOOL_INVOCATION"
	FrameRetry          TraceFrameType = "RETRY"
	FramePlanning       TraceFrameType = "PLANNING"
	FrameStepExecution  TraceFrameType = "STEP_EXECUTION"
)

// knownFrameType reports whether value is one of the current-release frame
// types.
func knownFrameType(value string) (TraceFrameType, bool) {
	ft := TraceFrameType(value)
	switch ft {
	case FrameRootMission, FrameSkillExecution, FrameModelCall, FrameToolInvocation,
		FrameRetry, FramePlanning, FrameStepExecution:
		return ft, true
	}
	return "", false
}

// TraceOutcome enumerates the canonical current-release terminal outcomes. These
// mirror com.lokiscale.loomspan.internal.core.TraceOutcome exactly.
type TraceOutcome string

const (
	OutcomeSucceeded TraceOutcome = "SUCCEEDED"
	OutcomeFailed    TraceOutcome = "FAILED"
	OutcomeAborted   TraceOutcome = "ABORTED"
)

// knownOutcome reports whether value is one of the current-release outcomes.
func knownOutcome(value string) (TraceOutcome, bool) {
	o := TraceOutcome(value)
	switch o {
	case OutcomeSucceeded, OutcomeFailed, OutcomeAborted:
		return o, true
	}
	return "", false
}

// UsagePrecision enumerates the canonical current-release usage precision
// values. These mirror com.lokiscale.loomspan.internal.runtime.usage.UsagePrecision
// exactly. ESTIMATED is intentionally absent: it is not a current production
// value and is rejected as UNSUPPORTED_VALUE.
type UsagePrecision string

const (
	PrecisionExact       UsagePrecision = "EXACT"
	PrecisionHeuristic   UsagePrecision = "HEURISTIC"
	PrecisionUnavailable UsagePrecision = "UNAVAILABLE"
)

// knownPrecision reports whether value is one of the current-release precision
// values.
func knownPrecision(value string) (UsagePrecision, bool) {
	p := UsagePrecision(value)
	switch p {
	case PrecisionExact, PrecisionHeuristic, PrecisionUnavailable:
		return p, true
	}
	return "", false
}
