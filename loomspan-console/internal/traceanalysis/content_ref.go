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

const contentKindSemantic contentKind = "SEMANTIC_CONTENT"

type contentSource string

const (
	contentSourceRecordData        contentSource = "RECORD_DATA"
	contentSourceEnvelope          contentSource = "RECONSTRUCTED_ENVELOPE"
	contentSourceFailureDiagnostic contentSource = "FAILURE_DIAGNOSTIC"
)

type contentReference struct {
	Schema        string          `json:"schema"`
	Source        evidence.Source `json:"source"`
	Handle        string          `json:"artifactHandle"`
	Kind          contentKind     `json:"kind"`
	ContentSource contentSource   `json:"contentSource"`
	Sequence      int64           `json:"sequence,omitempty"`
	PayloadID     string          `json:"payloadId,omitempty"`
	FailureID     string          `json:"failureId,omitempty"`
	Ordinal       *int            `json:"ordinal,omitempty"`
}

func encodeRecordContentReference(ref evidence.Reference, handle artifact.Handle, sequence int64) (string, error) {
	return encodeContentReferenceValue(contentReference{Schema: cursorSchemaV1, Source: ref.Source, Handle: string(handle), Kind: contentKindSemantic, ContentSource: contentSourceRecordData, Sequence: sequence})
}
func encodeEnvelopeContentReference(ref evidence.Reference, handle artifact.Handle, payloadID string) (string, error) {
	return encodeContentReferenceValue(contentReference{Schema: cursorSchemaV1, Source: ref.Source, Handle: string(handle), Kind: contentKindSemantic, ContentSource: contentSourceEnvelope, PayloadID: payloadID})
}
func encodeDiagnosticContentReference(ref evidence.Reference, handle artifact.Handle, failureID string, ordinal int) (string, error) {
	return encodeContentReferenceValue(contentReference{Schema: cursorSchemaV1, Source: ref.Source, Handle: string(handle), Kind: contentKindSemantic, ContentSource: contentSourceFailureDiagnostic, FailureID: failureID, Ordinal: &ordinal})
}
func encodeContentReferenceValue(value contentReference) (string, error) {
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

func contentReferenceIdentifierFits(id string) bool {
	_, err := encodeDiagnosticContentReference(evidence.ForImported(), artifact.Handle("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"), id, 15)
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
	var v contentReference
	if err := decoder.Decode(&v); err != nil {
		return contentReference{}, err
	}
	if decoder.Decode(&struct{}{}) == nil || v.Schema != cursorSchemaV1 || (v.Source != evidence.SourceTarget && v.Source != evidence.SourceImported) || v.Kind != contentKindSemantic {
		return contentReference{}, fmt.Errorf("content reference shape is unsupported")
	}
	switch v.ContentSource {
	case contentSourceRecordData:
		if v.Sequence <= 0 || v.PayloadID != "" || v.FailureID != "" || v.Ordinal != nil {
			return contentReference{}, fmt.Errorf("record content reference shape is invalid")
		}
	case contentSourceEnvelope:
		if v.PayloadID == "" || v.Sequence != 0 || v.FailureID != "" || v.Ordinal != nil {
			return contentReference{}, fmt.Errorf("envelope content reference shape is invalid")
		}
	case contentSourceFailureDiagnostic:
		if v.FailureID == "" || v.Sequence != 0 || v.PayloadID != "" || v.Ordinal == nil || *v.Ordinal < 0 {
			return contentReference{}, fmt.Errorf("diagnostic content reference shape is invalid")
		}
	default:
		return contentReference{}, fmt.Errorf("content reference source is unsupported")
	}
	return v, nil
}
func validateContentReference(v contentReference, ref evidence.Reference, handle artifact.Handle) error {
	if v.Source != ref.Source || v.Handle != string(handle) {
		return fmt.Errorf("content reference belongs to different evidence")
	}
	return nil
}
