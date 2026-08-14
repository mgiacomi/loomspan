package traceinventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const (
	cursorSchemaV1        = "v1"
	cursorOperation       = "TRACE_INVENTORY"
	maxContinuationLength = 8192
)

type cursorSegment string

const (
	segmentInstalled   cursorSegment = "INSTALLED"
	segmentApplication cursorSegment = "APPLICATION"
)

type installedKey struct {
	FinalizedAt time.Time `json:"finalizedAt"`
	Source      string    `json:"source"`
	TraceID     string    `json:"traceId"`
}

type inventoryCursor struct {
	Schema               string        `json:"schema"`
	Operation            string        `json:"operation"`
	Fingerprint          string        `json:"fingerprint"`
	Segment              cursorSegment `json:"segment"`
	Installed            *installedKey `json:"installed,omitempty"`
	InstalledFingerprint string        `json:"installedFingerprint,omitempty"`
	ApplicationCursor    string        `json:"applicationCursor,omitempty"`
}

func queryFingerprint(filter SourceFilter, pageSize int, scopeID string) string {
	body, _ := json.Marshal(struct {
		Filter   SourceFilter `json:"sourceFilter"`
		PageSize int          `json:"pageSize"`
		ScopeID  string       `json:"targetScopeId,omitempty"`
	}{filter, pageSize, scopeID})
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}

func encodeCursor(value inventoryCursor) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(body)
	if len(token) > maxContinuationLength {
		return "", fmt.Errorf("continuation exceeds %d characters", maxContinuationLength)
	}
	return token, nil
}

func decodeCursor(token string) (inventoryCursor, error) {
	if token == "" || len(token) > maxContinuationLength {
		return inventoryCursor{}, fmt.Errorf("continuation is empty or oversized")
	}
	body, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return inventoryCursor{}, fmt.Errorf("continuation is not unpadded base64url: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value inventoryCursor
	if err := decoder.Decode(&value); err != nil {
		return inventoryCursor{}, err
	}
	if decoder.Decode(&struct{}{}) == nil {
		return inventoryCursor{}, fmt.Errorf("continuation has trailing data")
	}
	if value.Schema != cursorSchemaV1 || value.Operation != cursorOperation ||
		(value.Segment != segmentInstalled && value.Segment != segmentApplication) {
		return inventoryCursor{}, fmt.Errorf("continuation shape is unsupported")
	}
	if value.Segment == segmentInstalled && value.Installed == nil {
		return inventoryCursor{}, fmt.Errorf("installed continuation key is missing")
	}
	if value.Segment == segmentApplication && value.Installed != nil {
		return inventoryCursor{}, fmt.Errorf("application continuation contains an installed key")
	}
	if value.Segment == segmentInstalled && (value.ApplicationCursor != "" || value.InstalledFingerprint != "") {
		return inventoryCursor{}, fmt.Errorf("installed continuation contains application state")
	}
	if value.Segment == segmentApplication && value.InstalledFingerprint == "" {
		return inventoryCursor{}, fmt.Errorf("application continuation is missing installed state")
	}
	return value, nil
}
