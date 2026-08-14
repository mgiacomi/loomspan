package mcpcredential

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func exerciseNativeCredentialOperations(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	if err := protectCredentialDirectory(directory); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(directory, CanonicalFileName)
	one := canonicalBytes([]byte("lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	two := canonicalBytes([]byte("lsmcp_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE"))

	prepared, err := prepareCredentialFile(directory, one)
	if err != nil {
		t.Fatal(err)
	}
	if result := commitCredentialEnable(prepared.path, canonical); result.err != nil || !result.changed {
		t.Fatalf("native exclusive enable = %+v", result)
	}
	assertNativeCredentialFile(t, canonical, one)

	conflict, err := prepareCredentialFile(directory, two)
	if err != nil {
		t.Fatal(err)
	}
	if result := commitCredentialEnable(conflict.path, canonical); result.err == nil {
		t.Fatal("native exclusive enable replaced an existing canonical file")
	}
	conflict.discard()
	assertNativeCredentialFile(t, canonical, one)

	replacement, err := prepareCredentialFile(directory, two)
	if err != nil {
		t.Fatal(err)
	}
	if result := commitCredentialReplace(replacement.path, canonical); result.err != nil || !result.changed {
		t.Fatalf("native replace = %+v", result)
	}
	assertNativeCredentialFile(t, canonical, two)

	if result := commitCredentialDelete(canonical); result.err != nil || !result.changed {
		t.Fatalf("native delete = %+v", result)
	}
	if _, err := os.Lstat(canonical); !os.IsNotExist(err) {
		t.Fatalf("canonical remains after native delete: %v", err)
	}
	if result := commitCredentialDelete(canonical); result.err == nil {
		t.Fatal("native delete unexpectedly accepted an absent canonical file")
	}
}

func assertNativeCredentialFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCredentialFile(path, info); err != nil {
		t.Fatalf("native credential protection: %v", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("native credential contents changed")
	}
}
