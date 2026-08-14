package traceanalysis

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

type contentKind string

const (
	contentKindPayload           contentKind = "PAYLOAD"
	contentKindFailureDiagnostic contentKind = "FAILURE_DIAGNOSTIC"
)

type contentReference struct {
	Schema    string          `json:"schema"`
	Source    evidence.Source `json:"source"`
	Handle    string          `json:"artifactHandle"`
	Kind      contentKind     `json:"kind"`
	PayloadID string          `json:"payloadId,omitempty"`
	FailureID string          `json:"failureId,omitempty"`
	Ordinal   *int            `json:"ordinal,omitempty"`
}

func encodeContentReference(ref evidence.Reference, handle artifact.Handle, kind contentKind, id string, ordinal *int) (string, error) {
	value := contentReference{Schema: cursorSchemaV1, Source: ref.Source, Handle: string(handle), Kind: kind, Ordinal: ordinal}
	if kind == contentKindPayload {
		value.PayloadID = id
	} else {
		value.FailureID = id
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(body)
	if len(token) > maxEncodedTokenLength {
		return "", fmt.Errorf("content reference is oversized")
	}
	return token, nil
}

// contentReferenceIdentifierFits reports whether an identifier can always be
// represented by the opaque content-reference contract. Imported is the
// longest source value and failure diagnostics have the largest envelope, so
// this probe is conservative for payload references as well.
func contentReferenceIdentifierFits(id string) bool {
	ordinal := 15 // failure diagnostics are limited to sixteen entries
	_, err := encodeContentReference(
		evidence.ForImported(),
		artifact.Handle("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
		contentKindFailureDiagnostic,
		id,
		&ordinal,
	)
	return err == nil
}

func decodeContentReference(token string) (contentReference, error) {
	if token == "" || len(token) > maxEncodedTokenLength {
		return contentReference{}, fmt.Errorf("content reference is empty or oversized")
	}
	body, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return contentReference{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value contentReference
	if err := decoder.Decode(&value); err != nil {
		return contentReference{}, err
	}
	if decoder.Decode(&struct{}{}) == nil || value.Schema != cursorSchemaV1 ||
		(value.Source != evidence.SourceTarget && value.Source != evidence.SourceImported) {
		return contentReference{}, fmt.Errorf("content reference shape is unsupported")
	}
	switch value.Kind {
	case contentKindPayload:
		if value.PayloadID == "" || value.FailureID != "" || value.Ordinal != nil {
			return contentReference{}, fmt.Errorf("payload reference shape is invalid")
		}
	case contentKindFailureDiagnostic:
		if value.FailureID == "" || value.PayloadID != "" || value.Ordinal == nil || *value.Ordinal < 0 {
			return contentReference{}, fmt.Errorf("diagnostic reference shape is invalid")
		}
	default:
		return contentReference{}, fmt.Errorf("content reference kind is unsupported")
	}
	return value, nil
}

func validateContentReference(value contentReference, ref evidence.Reference, handle artifact.Handle) error {
	if value.Source != ref.Source || value.Handle != string(handle) {
		return fmt.Errorf("content reference belongs to different evidence")
	}
	return nil
}
