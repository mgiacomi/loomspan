package mcpadapter

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

const maxContinuationLength = 8192

type continuationKind string

const (
	continuationSkills     continuationKind = "skills"
	continuationExecutions continuationKind = "executions"
	continuationActivity   continuationKind = "activity"
)

type continuationPayload struct {
	Version       int              `json:"version"`
	Kind          continuationKind `json:"kind"`
	TargetScopeID string           `json:"targetScopeId"`
	Cursor        string           `json:"cursor"`
	SessionID     string           `json:"sessionId,omitempty"`
}

func encodeContinuation(kind continuationKind, scopeID, cursor, sessionID string) (string, error) {
	payload := continuationPayload{Version: 1, Kind: kind, TargetScopeID: scopeID, Cursor: cursor, SessionID: sessionID}
	if err := validateContinuationPayload(payload); err != nil {
		return "", err
	}
	encodedJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode continuation: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(encodedJSON)
	if len(encoded) > maxContinuationLength {
		return "", fmt.Errorf("encoded continuation exceeds %d characters", maxContinuationLength)
	}
	return encoded, nil
}

func decodeContinuation(encoded string, expected continuationKind, currentScopeID, sessionID string) (string, *consolecore.Error) {
	invalid := func() (string, *consolecore.Error) {
		return "", consolecore.NewError(consolecore.CodeInvalidArgument, "The continuation is invalid.", currentScopeID, consolecore.Details{}, nil)
	}
	if encoded == "" || len(encoded) > maxContinuationLength || strings.Contains(encoded, "=") {
		return invalid()
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var payload continuationPayload
	if err := decoder.Decode(&payload); err != nil {
		return invalid()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalid()
	}
	if validateContinuationPayload(payload) != nil || payload.Kind != expected {
		return invalid()
	}
	if expected == continuationActivity {
		if !canonicalValue(sessionID) || payload.SessionID != sessionID {
			return invalid()
		}
	} else if payload.SessionID != "" {
		return invalid()
	}
	if payload.TargetScopeID != currentScopeID {
		return "", consolecore.NewError(
			consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			payload.TargetScopeID,
			consolecore.Details{CurrentTargetScopeID: currentScopeID},
			nil,
		)
	}
	return payload.Cursor, nil
}

func validateContinuationPayload(payload continuationPayload) error {
	if payload.Version != 1 {
		return fmt.Errorf("unsupported continuation version")
	}
	if payload.Kind != continuationSkills && payload.Kind != continuationExecutions && payload.Kind != continuationActivity {
		return fmt.Errorf("unsupported continuation kind")
	}
	if !canonicalValue(payload.TargetScopeID) || !canonicalValue(payload.Cursor) {
		return fmt.Errorf("continuation scope and cursor must be canonical nonblank values")
	}
	if payload.Kind == continuationActivity {
		if !canonicalValue(payload.SessionID) {
			return fmt.Errorf("activity continuation requires a canonical session ID")
		}
	} else if payload.SessionID != "" {
		return fmt.Errorf("only activity continuations may contain a session ID")
	}
	return nil
}

func canonicalValue(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}
