//go:build linux

package mcpcredential

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeLinuxLinkFallbackPublishesExclusively(t *testing.T) {
	directory := t.TempDir()
	if err := protectCredentialDirectory(directory); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(directory, CanonicalFileName)
	content := canonicalBytes([]byte("lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	prepared, err := prepareCredentialFile(directory, content)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := func(int, string, int, string, uint) error { return unix.ENOSYS }
	if result := commitCredentialEnableLinux(prepared.path, canonical, unsupported); result.err != nil || !result.changed {
		t.Fatalf("native link fallback = %+v", result)
	}
	assertNativeCredentialFile(t, canonical, content)
	if _, err := os.Lstat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("link fallback temporary remains: %v", err)
	}

	second, err := prepareCredentialFile(directory, content)
	if err != nil {
		t.Fatal(err)
	}
	result := commitCredentialEnableLinux(second.path, canonical, unsupported)
	if result.err == nil || result.changed || !errors.Is(result.err, os.ErrExist) {
		t.Fatalf("link fallback conflict = %+v", result)
	}
	second.discard()
}
