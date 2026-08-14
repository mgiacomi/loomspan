package mcpcredential

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func protectedDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := protectCredentialDirectory(directory); err != nil {
		t.Fatal(err)
	}
	return directory
}

var errInjectedDurability = errors.New("injected post-namespace durability failure")

func TestOpenStoreDistinguishesAbsentValidAndInvalidCanonicalState(t *testing.T) {
	directory := protectedDirectory(t)
	store, err := Open(directory, bytes.NewReader(make([]byte, 96)))
	if err != nil || store.Snapshot().State != Disabled {
		t.Fatalf("absent state = %+v, %v", store.Snapshot(), err)
	}
	prepared, err := store.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CommitEnable(prepared)
	if err != nil || key == "" || store.Snapshot().State != Enabled {
		t.Fatalf("enable = %q %+v %v", key, store.Snapshot(), err)
	}
	if _, ok := store.Authenticate(key); !ok {
		t.Fatal("committed key did not authenticate")
	}
	reopened, err := Open(directory, bytes.NewReader(make([]byte, 32)))
	if err != nil || reopened.Snapshot().State != Enabled {
		t.Fatalf("reopened = %+v, %v", reopened.Snapshot(), err)
	}
	if err := os.WriteFile(filepath.Join(directory, CanonicalFileName), []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid, err := Open(directory, bytes.NewReader(make([]byte, 32)))
	if err != nil || invalid.Snapshot().State != DisabledInvalid || invalid.Snapshot().Diagnostic == "" {
		t.Fatalf("invalid = %+v, %v", invalid.Snapshot(), err)
	}
	content, _ := os.ReadFile(filepath.Join(directory, CanonicalFileName))
	if string(content) != "invalid\n" {
		t.Fatal("invalid canonical file was repaired")
	}
}

func TestStoreEnableRegenerateDisable(t *testing.T) {
	entropy := make([]byte, 96)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	store, err := Open(protectedDirectory(t), bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := store.Prepare()
	oldKey, err := store.CommitEnable(one)
	if err != nil {
		t.Fatal(err)
	}
	two, _ := store.Prepare()
	newKey, err := store.CommitRegenerate(two)
	if err != nil || oldKey == newKey {
		t.Fatalf("regenerate error=%v", err)
	}
	if _, ok := store.Authenticate(oldKey); ok {
		t.Fatal("old key still authenticates")
	}
	if err := store.Disable(); err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().State != Disabled {
		t.Fatal("store did not disable")
	}
}

func TestOpenRemovesOnlyExactSafeCredentialTemporaryFiles(t *testing.T) {
	directory := protectedDirectory(t)
	safe := filepath.Join(directory, ".mcp-access-key-0123456789abcdef0123456789abcdef.tmp")
	nearMatch := filepath.Join(directory, ".mcp-access-key-0123456789abcdef0123456789abcdeg.tmp")
	if err := os.WriteFile(safe, canonicalBytes([]byte("lsmcp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := protectCredentialFile(safe); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nearMatch, []byte("leave me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(safe); !os.IsNotExist(err) {
		t.Fatalf("safe owned temporary file remains: %v", err)
	}
	if contents, err := os.ReadFile(nearMatch); err != nil || string(contents) != "leave me" {
		t.Fatalf("near-match file changed: %q %v", contents, err)
	}
}

func TestStoreReconcilesCanonicalAfterPostNamespaceFailures(t *testing.T) {
	entropy := make([]byte, 128)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	store, err := Open(protectedDirectory(t), bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	native := store.operations

	prepared, _ := store.Prepare()
	enabledKey := prepared.Credential()
	store.operations.enable = func(from, to string) namespaceMutation {
		if err := os.Rename(from, to); err != nil {
			return namespaceMutation{err: err}
		}
		return namespaceMutation{changed: true, err: errInjectedDurability}
	}
	if _, err := store.CommitEnable(prepared); !errors.Is(err, errInjectedDurability) {
		t.Fatalf("enable error = %v", err)
	}
	if store.Snapshot().State != Enabled {
		t.Fatalf("post-enable state = %+v", store.Snapshot())
	}
	if _, ok := store.Authenticate(enabledKey); !ok {
		t.Fatal("canonical enabled key was not published after post-namespace error")
	}

	prepared, _ = store.Prepare()
	replacementKey := prepared.Credential()
	store.operations.replace = func(from, to string) namespaceMutation {
		if err := os.Rename(from, to); err != nil {
			return namespaceMutation{err: err}
		}
		return namespaceMutation{changed: true, err: errInjectedDurability}
	}
	if _, err := store.CommitRegenerate(prepared); !errors.Is(err, errInjectedDurability) {
		t.Fatalf("regenerate error = %v", err)
	}
	if _, ok := store.Authenticate(replacementKey); !ok {
		t.Fatal("canonical replacement key was not published after post-namespace error")
	}
	if _, ok := store.Authenticate(enabledKey); ok {
		t.Fatal("old key remained accepted after canonical replacement")
	}

	store.operations.delete = func(path string) namespaceMutation {
		if err := os.Remove(path); err != nil {
			return namespaceMutation{err: err}
		}
		return namespaceMutation{changed: true, err: errInjectedDurability}
	}
	if err := store.Disable(); !errors.Is(err, errInjectedDurability) {
		t.Fatalf("disable error = %v", err)
	}
	if store.Snapshot().State != Disabled {
		t.Fatalf("post-delete state = %+v", store.Snapshot())
	}
	if _, ok := store.Authenticate(replacementKey); ok {
		t.Fatal("deleted canonical key remained accepted")
	}
	store.operations = native
}

func TestStorePreservesMemoryWhenNamespaceDefinitelyDidNotChange(t *testing.T) {
	store, err := Open(protectedDirectory(t), bytes.NewReader(make([]byte, 64)))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := store.Prepare()
	key, _ := store.CommitEnable(prepared)
	generation := store.Snapshot().Generation
	prepared, _ = store.Prepare()
	store.operations.replace = func(string, string) namespaceMutation {
		return namespaceMutation{err: errors.New("injected pre-namespace failure")}
	}
	if _, err := store.CommitRegenerate(prepared); err == nil {
		t.Fatal("pre-namespace failure was ignored")
	}
	if snapshot := store.Snapshot(); snapshot.State != Enabled || snapshot.Generation != generation {
		t.Fatalf("pre-namespace failure changed state: %+v", snapshot)
	}
	if _, ok := store.Authenticate(key); !ok {
		t.Fatal("pre-namespace failure replaced accepted key")
	}
}

func TestInvalidRemovalReconcilesAfterPostNamespaceFailure(t *testing.T) {
	directory := protectedDirectory(t)
	canonical := filepath.Join(directory, CanonicalFileName)
	if err := os.WriteFile(canonical, []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := protectCredentialFile(canonical); err != nil {
		t.Fatal(err)
	}
	store, err := Open(directory, nil)
	if err != nil || store.Snapshot().State != DisabledInvalid {
		t.Fatalf("open invalid state = %+v err=%v", store.Snapshot(), err)
	}
	store.operations.delete = func(path string) namespaceMutation {
		if err := os.Remove(path); err != nil {
			return namespaceMutation{err: err}
		}
		return namespaceMutation{changed: true, err: errInjectedDurability}
	}
	if err := store.RemoveInvalid(); !errors.Is(err, errInjectedDurability) {
		t.Fatalf("remove invalid error = %v", err)
	}
	if snapshot := store.Snapshot(); snapshot.State != Disabled || snapshot.Diagnostic != "" {
		t.Fatalf("post-removal state = %+v", snapshot)
	}
}
