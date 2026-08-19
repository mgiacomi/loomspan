package traceinventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

type inventoryCursor struct {
	Schema               string        `json:"schema"`
	Operation            string        `json:"operation"`
	Fingerprint          string        `json:"fingerprint"`
	Segment              cursorSegment `json:"segment"`
	InstalledOffset      int           `json:"installedOffset,omitempty"`
	InstalledFingerprint string        `json:"installedFingerprint"`
	ApplicationCursor    string        `json:"applicationCursor,omitempty"`
	ApplicationOffset    int           `json:"applicationOffset,omitempty"`
}

func queryFingerprint(query Query, pageSize int, scopeID string) string {
	copy := query
	copy.PageSize = pageSize
	copy.Continuation = ""
	body, _ := json.Marshal(struct {
		Query Query  `json:"query"`
		Scope string `json:"scope"`
	}{copy, scopeID})
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
	if value.Schema != cursorSchemaV1 || value.Operation != cursorOperation || (value.Segment != segmentInstalled && value.Segment != segmentApplication) {
		return inventoryCursor{}, fmt.Errorf("continuation shape is unsupported")
	}
	if value.InstalledFingerprint == "" || value.InstalledOffset < 0 || value.ApplicationOffset < 0 {
		return inventoryCursor{}, fmt.Errorf("continuation is missing installed state")
	}
	if value.Segment == segmentInstalled && value.ApplicationCursor != "" {
		return inventoryCursor{}, fmt.Errorf("installed continuation contains application state")
	}
	return value, nil
}
