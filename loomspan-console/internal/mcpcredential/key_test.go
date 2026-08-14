package mcpcredential

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateKeyUsesThirtyTwoRandomBytesAndExactCanonicalEncoding(t *testing.T) {
	key, err := generateKey(bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(key), "lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"; got != want {
		t.Fatalf("generated key shape differs")
	}
	canonical := canonicalBytes(key)
	if len(canonical) != 50 || canonical[49] != '\n' {
		t.Fatalf("canonical length/terminator = %d/%q", len(canonical), canonical[len(canonical)-1])
	}
	parsed, err := parseCanonical(canonical)
	if err != nil || !bytes.Equal(parsed, key) {
		t.Fatal("canonical key did not round trip")
	}
}

func TestParseCanonicalKeyRejectsEveryNonCanonicalVariant(t *testing.T) {
	valid := []byte("lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n")
	variants := [][]byte{
		nil, valid[:len(valid)-1], append(append([]byte(nil), valid...), '\n'),
		[]byte(strings.Replace(string(valid), "lsmcp_", "bfmcp_", 1)),
		[]byte(strings.Replace(string(valid), "A\n", "=\n", 1)),
		[]byte(strings.TrimSuffix(string(valid), "\n") + "\r\n"),
	}
	for _, variant := range variants {
		if _, err := parseCanonical(variant); err == nil {
			t.Fatalf("accepted noncanonical value of length %d", len(variant))
		}
	}
}
