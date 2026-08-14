package mcpcredential

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

type State string

const (
	Disabled        State = "DISABLED"
	Enabled         State = "ENABLED"
	DisabledInvalid State = "DISABLED_INVALID"
)

type Snapshot struct {
	State      State  `json:"state"`
	Generation uint64 `json:"-"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type Prepared struct {
	path string
	key  []byte
}

func (prepared *Prepared) Credential() string {
	if prepared == nil {
		return ""
	}
	return string(prepared.key)
}

func (prepared *Prepared) discard() {
	if prepared == nil {
		return
	}
	_ = os.Remove(prepared.path)
	clear(prepared.key)
	prepared.path = ""
}

type Store struct {
	mu         sync.RWMutex
	directory  string
	canonical  string
	entropy    io.Reader
	state      State
	diagnostic string
	key        []byte
	generation uint64
	invalid    os.FileInfo
	operations credentialOperations
}

var temporaryName = regexp.MustCompile(`^\.mcp-access-key-[0-9a-f]{32}\.tmp$`)

func Open(directory string, entropy io.Reader) (*Store, error) {
	if entropy == nil {
		entropy = rand.Reader
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP credential directory: %w", err)
	}
	if err := verifyCredentialDirectory(absolute); err != nil {
		return nil, fmt.Errorf("MCP credential directory protection: %w", err)
	}
	store := &Store{directory: absolute, canonical: filepath.Join(absolute, CanonicalFileName), entropy: entropy, state: Disabled, generation: 1, operations: nativeCredentialOperations()}
	if err := store.cleanupTemporaryFiles(); err != nil {
		return nil, err
	}
	store.inspectCanonical()
	return store, nil
}

func (store *Store) inspectCanonical() {
	info, err := os.Lstat(store.canonical)
	if os.IsNotExist(err) {
		store.state = Disabled
		return
	}
	if err != nil {
		store.markInvalid("canonical access key cannot be inspected", nil)
		return
	}
	if err := verifyCredentialFile(store.canonical, info); err != nil {
		store.markInvalid("canonical access key protection is invalid", info)
		return
	}
	content, err := readCanonical(store.canonical)
	if err != nil {
		store.markInvalid("canonical access key cannot be read", info)
		return
	}
	key, err := parseCanonical(content)
	clear(content)
	if err != nil {
		store.markInvalid("canonical access key format is invalid", info)
		return
	}
	store.state = Enabled
	store.key = key
}

func (store *Store) markInvalid(diagnostic string, info os.FileInfo) {
	store.state = DisabledInvalid
	store.diagnostic = diagnostic
	store.invalid = info
	clear(store.key)
	store.key = nil
}

func readCanonical(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, int64(canonicalLength+1)))
}

func (store *Store) Snapshot() Snapshot {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return Snapshot{State: store.state, Generation: store.generation, Diagnostic: store.diagnostic}
}

func (store *Store) Authenticate(credential string) (uint64, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.state != Enabled || len(credential) != len(store.key) {
		return store.generation, false
	}
	return store.generation, subtle.ConstantTimeCompare([]byte(credential), store.key) == 1
}

func (store *Store) Reveal() (string, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.state != Enabled {
		return "", fmt.Errorf("MCP is not enabled")
	}
	return string(store.key), nil
}

func (store *Store) Prepare() (*Prepared, error) {
	key, err := generateKey(store.entropy)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareCredentialFile(store.directory, canonicalBytes(key))
	if err != nil {
		clear(key)
		return nil, err
	}
	prepared.key = key
	return prepared, nil
}

func (store *Store) Discard(prepared *Prepared) { prepared.discard() }

func (store *Store) CommitEnable(prepared *Prepared) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != Disabled {
		prepared.discard()
		return "", fmt.Errorf("MCP can be enabled only from disabled state")
	}
	mutation := store.operations.enable(prepared.path, store.canonical)
	if mutation.err != nil {
		prepared.discard()
		if mutation.changed {
			store.reconcileCanonical()
		}
		return "", mutation.err
	}
	if err := store.publishPrepared(prepared); err != nil {
		store.inspectAfterFailedPublication()
		return "", err
	}
	return string(store.key), nil
}

func (store *Store) CommitRegenerate(prepared *Prepared) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != Enabled {
		prepared.discard()
		return "", fmt.Errorf("MCP is not enabled")
	}
	mutation := store.operations.replace(prepared.path, store.canonical)
	if mutation.err != nil {
		prepared.discard()
		if mutation.changed {
			store.reconcileCanonical()
		}
		return "", mutation.err
	}
	if err := store.publishPrepared(prepared); err != nil {
		store.inspectAfterFailedPublication()
		return "", err
	}
	return string(store.key), nil
}

func (store *Store) inspectAfterFailedPublication() {
	store.reconcileCanonical()
}

func (store *Store) reconcileCanonical() {
	clear(store.key)
	store.key = nil
	store.state = Disabled
	store.diagnostic = ""
	store.invalid = nil
	store.inspectCanonical()
	store.generation++
}

func (store *Store) publishPrepared(prepared *Prepared) error {
	info, err := os.Lstat(store.canonical)
	if err != nil || verifyCredentialFile(store.canonical, info) != nil {
		return fmt.Errorf("revalidate committed MCP access key")
	}
	content, err := readCanonical(store.canonical)
	if err != nil {
		return fmt.Errorf("revalidate committed MCP access key: %w", err)
	}
	key, err := parseCanonical(content)
	clear(content)
	if err != nil || subtle.ConstantTimeCompare(key, prepared.key) != 1 {
		clear(key)
		return fmt.Errorf("revalidate committed MCP access key")
	}
	clear(store.key)
	store.key = key
	store.state = Enabled
	store.diagnostic = ""
	store.invalid = nil
	store.generation++
	prepared.path = ""
	clear(prepared.key)
	return nil
}

func (store *Store) Disable() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != Enabled {
		return fmt.Errorf("MCP is not enabled")
	}
	mutation := store.operations.delete(store.canonical)
	if mutation.err != nil {
		if mutation.changed {
			store.reconcileCanonical()
		}
		return mutation.err
	}
	clear(store.key)
	store.key = nil
	store.state = Disabled
	store.generation++
	return nil
}

func (store *Store) RemoveInvalid() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != DisabledInvalid || store.invalid == nil {
		return fmt.Errorf("MCP access key is not in removable invalid state")
	}
	current, err := os.Lstat(store.canonical)
	if err != nil || !os.SameFile(store.invalid, current) || current.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid MCP access key changed; restart before removal")
	}
	mutation := store.operations.delete(store.canonical)
	if mutation.err != nil {
		if mutation.changed {
			store.reconcileCanonical()
		}
		return mutation.err
	}
	store.state = Disabled
	store.diagnostic = ""
	store.invalid = nil
	store.generation++
	return nil
}

func (store *Store) cleanupTemporaryFiles() error {
	entries, err := os.ReadDir(store.directory)
	if err != nil {
		return fmt.Errorf("inspect MCP credential temporary files: %w", err)
	}
	for _, entry := range entries {
		if !temporaryName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(store.directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || verifyCredentialFile(path, info) != nil {
			continue
		}
		if mutation := store.operations.delete(path); mutation.err != nil {
			return fmt.Errorf("remove safe MCP credential temporary file: %w", mutation.err)
		}
	}
	return nil
}
