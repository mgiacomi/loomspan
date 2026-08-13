package consolecore

import "fmt"

type Code string

const (
	CodeInvalidArgument           Code = "INVALID_ARGUMENT"
	CodeTargetAuthentication      Code = "TARGET_AUTHENTICATION_REQUIRED"
	CodeTargetAccessBlocked       Code = "TARGET_ACCESS_BLOCKED"
	CodeTargetUnavailable         Code = "TARGET_UNAVAILABLE"
	CodeIncompatibleTarget        Code = "INCOMPATIBLE_TARGET"
	CodeIncompatibleArtifact      Code = "INCOMPATIBLE_ARTIFACT"
	CodeTargetChanged             Code = "TARGET_CHANGED"
	CodeInvalidCursor             Code = "INVALID_CURSOR"
	CodeStaleCursor               Code = "STALE_CURSOR"
	CodeNotFound                  Code = "NOT_FOUND"
	CodeArtifactExpired           Code = "ARTIFACT_EXPIRED"
	CodeArtifactInUse             Code = "ARTIFACT_IN_USE"
	CodeArtifactAlreadyExists     Code = "ARTIFACT_ALREADY_EXISTS"
	CodeInvalidArtifact           Code = "INVALID_ARTIFACT"
	CodeLiveMonitoringUnavailable Code = "LIVE_MONITORING_UNAVAILABLE"
	CodeLimitExceeded             Code = "LIMIT_EXCEEDED"
	CodeLocalStorageUnavailable   Code = "LOCAL_STORAGE_UNAVAILABLE"
	CodeConsoleError              Code = "CONSOLE_ERROR"
)

type Details struct {
	ExpectedCompatibilityVersion string `json:"expectedCompatibilityVersion,omitempty"`
	ObservedCompatibilityVersion string `json:"observedCompatibilityVersion,omitempty"`
	CurrentTargetScopeID         string `json:"currentTargetScopeId,omitempty"`
	TransportCategory            string `json:"transportCategory,omitempty"`
	LimitName                    string `json:"limitName,omitempty"`
	LimitValue                   int64  `json:"limitValue,omitempty"`
	RawDownloadAvailable         *bool  `json:"rawDownloadAvailable,omitempty"`
}

type Error struct {
	Code          Code
	Message       string
	TargetScopeID string
	Details       Details
	cause         error
}

func NewError(code Code, message, scope string, details Details, cause error) *Error {
	if message == "" || len(message) > 512 {
		message = "The Console operation could not be completed."
	}
	return &Error{Code: code, Message: message, TargetScopeID: scope, Details: details, cause: cause}
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	return err.Message
}

func (err *Error) Unwrap() error { return err.cause }

func (err *Error) GoString() string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("console error %s: %s", err.Code, err.Message)
}
