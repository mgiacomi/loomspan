package mcpcredential

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// namespaceMutation reports whether the canonical namespace may already have
// changed when err is non-nil. Callers must re-inspect disk whenever changed is
// true instead of retaining a stale in-memory credential state.
type namespaceMutation struct {
	changed bool
	err     error
}

type credentialOperations struct {
	enable  func(string, string) namespaceMutation
	replace func(string, string) namespaceMutation
	delete  func(string) namespaceMutation
}

func nativeCredentialOperations() credentialOperations {
	return credentialOperations{
		enable:  commitCredentialEnable,
		replace: commitCredentialReplace,
		delete:  commitCredentialDelete,
	}
}

func prepareCredentialFile(directory string, content []byte) (*Prepared, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("name MCP credential temporary file: %w", err)
	}
	path := filepath.Join(directory, ".mcp-access-key-"+hex.EncodeToString(nonce)+".tmp")
	clear(nonce)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create MCP credential temporary file: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if err := protectCredentialFile(path); err != nil {
		return nil, fmt.Errorf("protect MCP credential temporary file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return nil, fmt.Errorf("write MCP credential temporary file: %w", err)
	}
	if err := flushCredentialFile(file); err != nil {
		return nil, fmt.Errorf("flush MCP credential temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close MCP credential temporary file: %w", err)
	}
	remove = false
	return &Prepared{path: path}, nil
}
