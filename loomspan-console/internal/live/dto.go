package live

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

const (
	maxSummaryCodePoints = 512
	maxActivityUTF8Bytes = 12 * 1024
)

type ActivityKind string

const (
	KindTraceStarted              ActivityKind = "TRACE_STARTED"
	KindFrameOpened               ActivityKind = "FRAME_OPENED"
	KindFrameClosed               ActivityKind = "FRAME_CLOSED"
	KindModelRequestSent          ActivityKind = "MODEL_REQUEST_SENT"
	KindModelResponseReceived     ActivityKind = "MODEL_RESPONSE_RECEIVED"
	KindModelAttemptFailed        ActivityKind = "MODEL_ATTEMPT_FAILED"
	KindPlanCreated               ActivityKind = "PLAN_CREATED"
	KindPlanUpdated               ActivityKind = "PLAN_UPDATED"
	KindPlanValidationFailed      ActivityKind = "PLAN_VALIDATION_FAILED"
	KindPlanRetryRequested        ActivityKind = "PLAN_RETRY_REQUESTED"
	KindToolCallStarted           ActivityKind = "TOOL_CALL_STARTED"
	KindToolCallCompleted         ActivityKind = "TOOL_CALL_COMPLETED"
	KindToolCallFailed            ActivityKind = "TOOL_CALL_FAILED"
	KindStepStarted               ActivityKind = "STEP_STARTED"
	KindStepActionRejected        ActivityKind = "STEP_ACTION_REJECTED"
	KindStepCompleted             ActivityKind = "STEP_COMPLETED"
	KindErrorRecorded             ActivityKind = "ERROR_RECORDED"
	KindTraceCompleted            ActivityKind = "TRACE_COMPLETED"
	KindExecutionObservationEnded ActivityKind = "EXECUTION_OBSERVATION_ENDED"
)

var allKinds = map[ActivityKind]bool{
	KindTraceStarted: true, KindFrameOpened: true, KindFrameClosed: true,
	KindModelRequestSent: true, KindModelResponseReceived: true,
	KindPlanCreated: true, KindPlanUpdated: true, KindPlanValidationFailed: true,
	KindPlanRetryRequested: true, KindToolCallStarted: true, KindToolCallCompleted: true,
	KindToolCallFailed: true, KindStepStarted: true, KindStepActionRejected: true,
	KindStepCompleted: true, KindErrorRecorded: true, KindTraceCompleted: true,
	KindExecutionObservationEnded: true,
}

func IsValidKind(kind ActivityKind) bool {
	return allKinds[kind]
}

func KindLabels() map[ActivityKind]string {
	return map[ActivityKind]string{
		KindTraceStarted:              "Execution started",
		KindFrameOpened:               "Skill execution started",
		KindFrameClosed:               "Skill execution completed",
		KindModelRequestSent:          "Model request sent",
		KindModelResponseReceived:     "Model response received",
		KindPlanCreated:               "Plan created",
		KindPlanUpdated:               "Plan updated",
		KindPlanValidationFailed:      "Plan validation failed",
		KindPlanRetryRequested:        "Plan retry requested",
		KindToolCallStarted:           "Tool call started",
		KindToolCallCompleted:         "Tool call completed",
		KindToolCallFailed:            "Tool call failed",
		KindStepStarted:               "Step started",
		KindStepActionRejected:        "Step action rejected",
		KindStepCompleted:             "Step completed",
		KindErrorRecorded:             "Execution error recorded",
		KindTraceCompleted:            "Execution completed",
		KindExecutionObservationEnded: "Execution observation ended",
	}
}

type Handshake struct {
	InstanceID  string    `json:"instanceId"`
	ObservedAt  time.Time `json:"observedAt"`
	AfterCursor string    `json:"afterCursor"`
}

type Activity struct {
	InstanceID        string          `json:"instanceId"`
	Cursor            string          `json:"cursor"`
	SessionID         string          `json:"sessionId"`
	TraceID           string          `json:"traceId"`
	CanonicalSequence *int64          `json:"canonicalSequence,omitempty"`
	Timestamp         time.Time       `json:"timestamp"`
	Kind              ActivityKind    `json:"kind"`
	ExecutionStatus   string          `json:"executionStatus,omitempty"`
	FrameID           string          `json:"frameId,omitempty"`
	ParentFrameID     string          `json:"parentFrameId,omitempty"`
	FrameType         string          `json:"frameType,omitempty"`
	Route             string          `json:"route,omitempty"`
	Summary           string          `json:"summary"`
	Details           json.RawMessage `json:"details"`
}

func (a *Activity) Validate() error {
	if a.InstanceID == "" {
		return fmt.Errorf("instanceId must not be blank")
	}
	if a.Cursor == "" {
		return fmt.Errorf("cursor must not be blank")
	}
	cursor, err := strconv.ParseUint(a.Cursor, 10, 64)
	if err != nil || cursor == 0 {
		return fmt.Errorf("cursor must be a positive decimal integer")
	}
	if a.SessionID == "" {
		return fmt.Errorf("sessionId must not be blank")
	}
	if a.TraceID == "" {
		return fmt.Errorf("traceId must not be blank")
	}
	if a.CanonicalSequence != nil && *a.CanonicalSequence <= 0 {
		return fmt.Errorf("canonicalSequence must be positive when present")
	}
	if a.Timestamp.IsZero() {
		return fmt.Errorf("timestamp must not be zero")
	}
	if !IsValidKind(a.Kind) {
		return fmt.Errorf("unknown activity kind: %s", a.Kind)
	}
	if len([]rune(a.Summary)) > maxSummaryCodePoints {
		return fmt.Errorf("summary exceeds %d code points", maxSummaryCodePoints)
	}
	if len(a.Details) == 0 || !json.Valid(a.Details) {
		return fmt.Errorf("details must contain valid JSON")
	}
	return nil
}

func (a *Activity) EncodedSize() int {
	data, err := json.Marshal(a)
	if err != nil {
		return 0
	}
	return len(data)
}

type ResetCause string

const (
	ResetTargetScopeChanged  ResetCause = "target_scope_changed"
	ResetInstanceChanged     ResetCause = "instance_changed"
	ResetUpstreamStaleCursor ResetCause = "upstream_stale_cursor"
	ResetShutdown            ResetCause = "shutdown"
)

type ResetFact struct {
	Cause     ResetCause `json:"cause"`
	Timestamp time.Time  `json:"timestamp"`
	Cursor    string     `json:"cursor,omitempty"`
}

type Continuity struct {
	IntervalID    string     `json:"intervalId"`
	TargetScopeID string     `json:"targetScopeId"`
	InstanceID    string     `json:"instanceId"`
	FirstCursor   string     `json:"firstCursor,omitempty"`
	LastCursor    string     `json:"lastCursor,omitempty"`
	ObservedAt    time.Time  `json:"observedAt,omitempty"`
	Reset         *ResetFact `json:"reset,omitempty"`
}

type ConnectionFact struct {
	Connected bool      `json:"connected"`
	Reason    string    `json:"reason,omitempty"`
	At        time.Time `json:"at,omitempty"`
}

type RecentRequest struct {
	Cursor    string `json:"cursor,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type RecentResponse struct {
	Items                []Activity  `json:"items"`
	HasMore              bool        `json:"hasMore"`
	NextCursor           string      `json:"nextCursor"`
	Continuity           *Continuity `json:"continuity,omitempty"`
	BeginningUnavailable bool        `json:"beginningUnavailable"`
}
