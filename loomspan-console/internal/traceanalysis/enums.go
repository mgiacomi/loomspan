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

// RecordTypeValues returns the closed current-release vocabulary used by
// service validation and outward adapter schemas.
func RecordTypeValues() []string {
	return []string{
		string(RecordTraceStarted), string(RecordTraceCapturePolicy), string(RecordFrameOpened), string(RecordFrameMetadata),
		string(RecordPayloadChunkAppended), string(RecordModelRequestPrepared), string(RecordModelRequestSent),
		string(RecordAdvisorRequestMutation), string(RecordModelResponseReceived), string(RecordModelAttemptFailed),
		string(RecordAdvisorResponseMutation), string(RecordModelThoughtCaptured), string(RecordPlanCreated), string(RecordPlanUpdated),
		string(RecordPlanValidationFailed), string(RecordPlanRetryRequested), string(RecordPlanQualityWarning),
		string(RecordToolCallStarted), string(RecordToolCallCompleted), string(RecordToolCallFailed), string(RecordEvidenceRecorded),
		string(RecordEvidenceValidationFailed), string(RecordEvidenceValidationPassed), string(RecordLinterRecorded),
		string(RecordStructuredOutputRecorded), string(RecordStepStarted), string(RecordStepActionProposed),
		string(RecordStepActionValidated), string(RecordStepActionRejected), string(RecordStepCompleted), string(RecordErrorRecorded),
		string(RecordFrameClosed), string(RecordTraceCompleted),
	}
}

// knownRecordType reports whether value is one of the current-release record
// types.
func knownRecordType(value string) (TraceRecordType, bool) {
	if containsClosedValue(RecordTypeValues(), value) {
		return TraceRecordType(value), true
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

func FrameTypeValues() []string {
	return []string{string(FrameRootMission), string(FrameSkillExecution), string(FrameModelCall), string(FrameToolInvocation), string(FrameRetry), string(FramePlanning), string(FrameStepExecution)}
}

// knownFrameType reports whether value is one of the current-release frame
// types.
func knownFrameType(value string) (TraceFrameType, bool) {
	if containsClosedValue(FrameTypeValues(), value) {
		return TraceFrameType(value), true
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

func TraceOutcomeValues() []string {
	return []string{string(OutcomeSucceeded), string(OutcomeFailed), string(OutcomeAborted)}
}

func FrameOutcomeValues() []string { return []string{"completed", "failed", "aborted"} }

func ValidationStatusValues() []string { return []string{"retrying", "passed", "exhausted"} }

func FrameOrderValues() []string {
	return []string{string(FrameOrderCanonical), string(FrameOrderDurationDesc), string(FrameOrderUsageDesc)}
}

func FrameProjectionValues() []string {
	return []string{string(FrameProjectionCompact), string(FrameProjectionDetailed)}
}

func RecordRepresentationValues() []string {
	return []string{string(RecordRepresentationLogical), string(RecordRepresentationPhysical)}
}

// knownOutcome reports whether value is one of the current-release outcomes.
func knownOutcome(value string) (TraceOutcome, bool) {
	if containsClosedValue(TraceOutcomeValues(), value) {
		return TraceOutcome(value), true
	}
	return "", false
}

func containsClosedValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
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
