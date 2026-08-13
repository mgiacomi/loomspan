package traceanalysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// cursorSchemaV1 is the version tag carried by every continuation cursor. It
// lets a future phase evolve the cursor shape without a durable compatibility
// reader; an unknown schema is rejected as INVALID_CURSOR.
const cursorSchemaV1 = "v1"

// cursorOp identifies which query operation a cursor continues.
type cursorOp string

const (
	cursorOpFrames           cursorOp = "FRAMES"
	cursorOpRecords          cursorOp = "RECORDS"
	cursorOpAttempts         cursorOp = "ATTEMPTS"
	cursorOpRetries          cursorOp = "RETRIES"
	cursorOpValidation       cursorOp = "VALIDATION"
	cursorOpFailures         cursorOp = "FAILURES"
	cursorOpUsage            cursorOp = "USAGE"
	cursorOpPayloads         cursorOp = "PAYLOADS"
	cursorOpGaps             cursorOp = "GAPS"
	cursorOpUncertainty      cursorOp = "UNCERTAINTY"
	cursorOpSearch           cursorOp = "SEARCH"
	cursorOpPayloadRange     cursorOp = "PAYLOAD_RANGE"
	cursorOpRawRecordRange   cursorOp = "RAW_RECORD_RANGE"
	cursorOpRawArtifactRange cursorOp = "RAW_ARTIFACT_RANGE"
)

// cursor is the internal, versioned continuation token. It is opaque to
// callers: encode/decode use URL-safe base64 with strict unknown/trailing-field
// rejection. No signature is required because the loopback APIs are
// independently authenticated.
type cursor struct {
	Schema      string   `json:"schema"`
	Op          cursorOp `json:"op"`
	OwnerKey    string   `json:"ownerKey"`
	Handle      string   `json:"handle"`
	Fingerprint string   `json:"fingerprint"`
	// Position is the next record sequence, fact index, or byte offset,
	// depending on the operation.
	Position int64 `json:"position"`
	// SearchState carries optional intra-payload KMP state for search
	// continuation so an oversized single payload can make bounded progress
	// without missing a boundary-spanning literal.
	SearchState *searchCursorState `json:"searchState,omitempty"`
}

// searchCursorState carries exact byte progress through the physical-record
// and reconstructed-payload search phases. ByteOffset and KMPPartial let a
// call stop inside a large value without losing a boundary-spanning match.
type searchCursorState struct {
	Phase           string `json:"phase"`
	RecordPosition  int64  `json:"recordPosition"`
	PayloadPosition int64  `json:"payloadPosition"`
	// PayloadIndexOffset is the length-prefixed row offset for
	// PayloadPosition. It lets payload continuations seek directly to the
	// current descriptor, including when ByteOffset stops inside its payload.
	PayloadIndexOffset int64 `json:"payloadIndexOffset"`
	ByteOffset         int64 `json:"byteOffset"`
	KMPPartial         int   `json:"kmpPartial"`
}

// canonicalizeRequest produces a deterministic JSON encoding of the query
// request fields that define the operation's meaning. The encoding is hashed
// with SHA-256 to form the cursor fingerprint. Any change to the filter, order,
// representation, range, or inline-payload request produces a different
// fingerprint so a cursor cannot be reused with a different query meaning.
func canonicalizeRequest(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	// Normalize through a round-trip so map key ordering is deterministic.
	var normalized any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&normalized); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", sum[:]), nil
}

// encodeCursor serializes a cursor to URL-safe base64.
func encodeCursor(c cursor) (string, error) {
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64URLEncode(body), nil
}

// decodeCursor parses a URL-safe base64 cursor and rejects unknown schema,
// trailing fields, or malformed bytes. It does NOT validate scope/handle
// lifetime; that is the caller's responsibility and follows the
// TARGET_CHANGED -> ARTIFACT_EXPIRED -> INVALID_CURSOR precedence.
func decodeCursor(token string) (cursor, error) {
	if token == "" {
		return cursor{}, errors.New("cursor is empty")
	}
	body, err := base64URLDecode(token)
	if err != nil {
		return cursor{}, fmt.Errorf("cursor is not valid base64: %w", err)
	}
	// Reject unknown/trailing fields by decoding into a strict struct. JSON
	// unmarshal into a struct ignores unknown fields by default, so we also
	// re-decode into a generic map and reject any key not in the cursor schema.
	var c cursor
	if err := json.Unmarshal(body, &c); err != nil {
		return cursor{}, fmt.Errorf("cursor is malformed: %w", err)
	}
	if c.Schema != cursorSchemaV1 {
		return cursor{}, fmt.Errorf("cursor schema %q is not supported", c.Schema)
	}
	if c.Op == "" {
		return cursor{}, errors.New("cursor operation is missing")
	}
	if c.OwnerKey == "" || c.Handle == "" || c.Fingerprint == "" {
		return cursor{}, errors.New("cursor is missing required fields")
	}
	if c.Position < 0 {
		return cursor{}, errors.New("cursor position is negative")
	}
	if c.SearchState != nil && (c.SearchState.RecordPosition < 0 || c.SearchState.PayloadPosition < 0 ||
		c.SearchState.PayloadIndexOffset < 0 ||
		c.SearchState.ByteOffset < 0 || c.SearchState.KMPPartial < 0 ||
		(c.SearchState.Phase != "records" && c.SearchState.Phase != "payloads")) {
		return cursor{}, errors.New("cursor search state contains negative progress")
	}
	// Reject unknown fields.
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return cursor{}, fmt.Errorf("cursor is malformed: %w", err)
	}
	allowed := map[string]bool{
		"schema": true, "op": true, "ownerKey": true, "handle": true,
		"fingerprint": true, "position": true, "searchState": true,
	}
	for k := range raw {
		if !allowed[k] {
			return cursor{}, fmt.Errorf("cursor has unknown field %q", k)
		}
	}
	return c, nil
}

// validateCursorPrecedence checks the cursor against the current scope, handle,
// and fingerprint in the required precedence order:
//  1. TARGET_CHANGED if the cursor's scope is not the current scope.
//  2. ARTIFACT_EXPIRED if the handle is no longer installed in the current scope.
//  3. INVALID_CURSOR if the fingerprint does not match the current request.
//
// The caller must already hold the lease (so the handle is installed) before
// calling this; the lease acquisition itself enforces steps 1 and 2. This
// helper performs the fingerprint check and is used after a successful lease
// acquisition to validate the cursor's query meaning.
func validateCursorFingerprint(c cursor, expectedFingerprint, ownerKey, errorScope string, handle artifact.Handle) *consolecore.Error {
	if c.OwnerKey != ownerKey {
		return consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			errorScope, consolecore.Details{}, nil)
	}
	if c.Handle != string(handle) || c.Fingerprint != expectedFingerprint {
		return consolecore.NewError(consolecore.CodeInvalidCursor,
			"The continuation does not match this query.", errorScope, consolecore.Details{}, nil)
	}
	return nil
}

// cursorError maps a malformed/tampered cursor to INVALID_CURSOR.
func cursorError(scopeID string, cause error) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeInvalidCursor,
		"The continuation is malformed.", scopeID, consolecore.Details{}, cause)
}

// prepareCursor validates token shape, operation, and recorded scope after the
// caller has acquired the current-scope artifact lease.
//  1. INVALID_CURSOR — malformed token or wrong operation type.
//  2. TARGET_CHANGED — cursor's scope is not the current scope.
//
// Lease acquisition enforces TARGET_CHANGED, then ARTIFACT_EXPIRED, before
// malformed or mismatched cursor state is reported as INVALID_CURSOR.
func prepareCursor(token, ownerKey, errorScope string, expectedOp cursorOp) (cursor, int, *consolecore.Error) {
	c, err := decodeCursor(token)
	if err != nil {
		return cursor{}, 0, cursorError(errorScope, err)
	}
	if c.Op != expectedOp {
		return cursor{}, 0, cursorError(errorScope, errCursorOpMismatch)
	}
	if c.OwnerKey != ownerKey {
		return cursor{}, 0, consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			errorScope, consolecore.Details{}, nil)
	}
	return c, int(c.Position), nil
}

// base64URLEncode encodes bytes using URL-safe base64 without padding.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64URLDecode decodes URL-safe base64 without padding.
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// encodePositionCursor builds a simple position-based continuation cursor.
func encodePositionCursor(op cursorOp, ownerKey string, handle artifact.Handle, fingerprint string, position int64) (string, error) {
	return encodeCursor(cursor{
		Schema:      cursorSchemaV1,
		Op:          op,
		OwnerKey:    ownerKey,
		Handle:      string(handle),
		Fingerprint: fingerprint,
		Position:    position,
	})
}

// encodeSearchCursor builds a search continuation cursor carrying KMP state.
func encodeSearchCursor(ownerKey string, handle artifact.Handle, fingerprint string, state searchCursorState) (string, error) {
	return encodeCursor(cursor{
		Schema:      cursorSchemaV1,
		Op:          cursorOpSearch,
		OwnerKey:    ownerKey,
		Handle:      string(handle),
		Fingerprint: fingerprint,
		SearchState: &state,
	})
}

// encodeRangeCursor builds a byte-range continuation cursor. Position is the
// next byte offset to read.
func encodeRangeCursor(op cursorOp, ownerKey string, handle artifact.Handle, fingerprint string, nextOffset int64) (string, error) {
	return encodeCursor(cursor{
		Schema:      cursorSchemaV1,
		Op:          op,
		OwnerKey:    ownerKey,
		Handle:      string(handle),
		Fingerprint: fingerprint,
		Position:    nextOffset,
	})
}

func ownerCursorKey(owner evidence.Owner) string {
	return string(owner.Source()) + ":" + owner.ID()
}
