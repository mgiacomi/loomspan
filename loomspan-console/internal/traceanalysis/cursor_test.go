package traceanalysis

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
)

func TestCursorEncodeDecodeRoundTrip(t *testing.T) {
	c := cursor{
		Schema:      cursorSchemaV1,
		Op:          cursorOpFrames,
		OwnerKey:    "scope-1",
		Handle:      "handle-1",
		Fingerprint: "abc123",
		Position:    42,
	}
	encoded, err := encodeCursor(c)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if encoded == "" {
		t.Fatal("expected non-empty encoded cursor")
	}
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.Schema != c.Schema {
		t.Fatalf("schema mismatch: got %q want %q", decoded.Schema, c.Schema)
	}
	if decoded.Op != c.Op {
		t.Fatalf("op mismatch: got %q want %q", decoded.Op, c.Op)
	}
	if decoded.OwnerKey != c.OwnerKey {
		t.Fatalf("scopeID mismatch: got %q want %q", decoded.OwnerKey, c.OwnerKey)
	}
	if decoded.Handle != c.Handle {
		t.Fatalf("handle mismatch: got %q want %q", decoded.Handle, c.Handle)
	}
	if decoded.Fingerprint != c.Fingerprint {
		t.Fatalf("fingerprint mismatch: got %q want %q", decoded.Fingerprint, c.Fingerprint)
	}
	if decoded.Position != c.Position {
		t.Fatalf("position mismatch: got %d want %d", decoded.Position, c.Position)
	}
}

func TestCursorRejectsUnknownSchema(t *testing.T) {
	c := cursor{
		Schema:      "v999",
		Op:          cursorOpFrames,
		OwnerKey:    "scope-1",
		Handle:      "handle-1",
		Fingerprint: "abc123",
		Position:    1,
	}
	encoded, err := encodeCursor(c)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if _, err := decodeCursor(encoded); err == nil {
		t.Fatal("expected error for unknown schema, got nil")
	}
}

func TestCursorRejectsEmptyToken(t *testing.T) {
	if _, err := decodeCursor(""); err == nil {
		t.Fatal("expected error for empty cursor, got nil")
	}
}

func TestCursorRejectsMalformedBase64(t *testing.T) {
	if _, err := decodeCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for malformed base64, got nil")
	}
}

func TestCursorRejectsUnknownField(t *testing.T) {
	// Build a cursor JSON with an extra unknown field.
	raw := map[string]any{
		"schema":      cursorSchemaV1,
		"op":          "FRAMES",
		"ownerKey":    "scope-1",
		"handle":      "handle-1",
		"fingerprint": "abc123",
		"position":    5,
		"evil":        true,
	}
	body, _ := json.Marshal(raw)
	encoded := base64URLEncode(body)
	if _, err := decodeCursor(encoded); err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestCursorRejectsMissingRequiredFields(t *testing.T) {
	c := cursor{
		Schema: cursorSchemaV1,
		Op:     cursorOpFrames,
		// Missing ScopeID, Handle, Fingerprint.
	}
	encoded, err := encodeCursor(c)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	if _, err := decodeCursor(encoded); err == nil {
		t.Fatal("expected error for missing required fields, got nil")
	}
}

func TestCursorSearchStateRoundTrip(t *testing.T) {
	c := cursor{
		Schema:      cursorSchemaV1,
		Op:          cursorOpSearch,
		OwnerKey:    "scope-1",
		Handle:      "handle-1",
		Fingerprint: "def456",
		SearchState: &searchCursorState{
			Phase:          "payloads",
			RecordPosition: 10,
			ByteOffset:     1000,
			KMPPartial:     3,
		},
	}
	encoded, err := encodeCursor(c)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.SearchState == nil {
		t.Fatal("expected non-nil search state")
	}
	if decoded.SearchState.RecordPosition != 10 {
		t.Fatalf("recordPosition mismatch: got %d want 10", decoded.SearchState.RecordPosition)
	}
	if decoded.SearchState.KMPPartial != 3 {
		t.Fatalf("kmpPartial mismatch: got %d want 3", decoded.SearchState.KMPPartial)
	}
}

func TestCanonicalizeRequestDeterministic(t *testing.T) {
	a := struct {
		Filter string `json:"filter"`
		Order  string `json:"order"`
	}{
		Filter: "test",
		Order:  "CANONICAL",
	}
	fp1, err := canonicalizeRequest(a)
	if err != nil {
		t.Fatalf("canonicalize failed: %v", err)
	}
	fp2, err := canonicalizeRequest(a)
	if err != nil {
		t.Fatalf("canonicalize failed: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("canonicalization is not deterministic: %q vs %q", fp1, fp2)
	}
}

func TestCanonicalizeRequestDifferentForDifferentInput(t *testing.T) {
	a := struct {
		Filter string `json:"filter"`
		Order  string `json:"order"`
	}{
		Filter: "test",
		Order:  "CANONICAL",
	}
	b := struct {
		Filter string `json:"filter"`
		Order  string `json:"order"`
	}{
		Filter: "test",
		Order:  "DURATION_DESC",
	}
	fpA, _ := canonicalizeRequest(a)
	fpB, _ := canonicalizeRequest(b)
	if fpA == fpB {
		t.Fatal("expected different fingerprints for different inputs")
	}
}

func TestValidateCursorFingerprintScopeChanged(t *testing.T) {
	c := cursor{
		OwnerKey:    "scope-old",
		Fingerprint: "abc123",
	}
	domain := validateCursorFingerprint(c, "abc123", "scope-new", "scope-new", artifact.Handle(c.Handle))
	if domain == nil {
		t.Fatal("expected TARGET_CHANGED error")
	}
	if domain.Code != "TARGET_CHANGED" {
		t.Fatalf("expected TARGET_CHANGED, got %s", domain.Code)
	}
}

func TestValidateCursorFingerprintMismatch(t *testing.T) {
	c := cursor{
		OwnerKey:    "scope-1",
		Fingerprint: "abc123",
	}
	domain := validateCursorFingerprint(c, "def456", "scope-1", "scope-1", artifact.Handle(c.Handle))
	if domain == nil {
		t.Fatal("expected INVALID_CURSOR error")
	}
	if domain.Code != "INVALID_CURSOR" {
		t.Fatalf("expected INVALID_CURSOR, got %s", domain.Code)
	}
}

func TestValidateCursorFingerprintOK(t *testing.T) {
	c := cursor{
		OwnerKey:    "scope-1",
		Fingerprint: "abc123",
	}
	if domain := validateCursorFingerprint(c, "abc123", "scope-1", "scope-1", artifact.Handle(c.Handle)); domain != nil {
		t.Fatalf("expected no error, got %v", domain)
	}
}

func TestBase64URLCodec(t *testing.T) {
	tests := [][]byte{
		{},
		{0},
		{0, 1},
		{0, 1, 2},
		{255},
		{255, 254, 253},
		[]byte("hello world"),
		bytesRepeat(0xAB, 100),
	}
	for _, data := range tests {
		encoded := base64URLEncode(data)
		decoded, err := base64URLDecode(encoded)
		if err != nil {
			t.Fatalf("decode failed for %v: %v", data, err)
		}
		if !bytesEqual(decoded, data) {
			t.Fatalf("round-trip mismatch: got %v want %v", decoded, data)
		}
	}
}

func TestEncodePositionCursor(t *testing.T) {
	encoded, err := encodePositionCursor(cursorOpFrames, "scope-1", artifact.Handle("handle-1"), "fp123", 42)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if decoded.Op != cursorOpFrames {
		t.Fatalf("op mismatch: got %q want %q", decoded.Op, cursorOpFrames)
	}
	if decoded.Position != 42 {
		t.Fatalf("position mismatch: got %d want 42", decoded.Position)
	}
}

func TestCursorOpMismatchError(t *testing.T) {
	if errCursorOpMismatch.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
	if !strings.Contains(errCursorOpMismatch.Error(), "cursor") {
		t.Fatalf("expected error to mention cursor, got %q", errCursorOpMismatch.Error())
	}
}

// bytesRepeat returns a byte slice of length n with each byte set to v.
func bytesRepeat(v byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// bytesEqual reports whether a and b are byte-for-byte equal.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
