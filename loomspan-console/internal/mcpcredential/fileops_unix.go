//go:build !windows

package mcpcredential

import (
	"fmt"
	"os"
	"syscall"
)

func verifyCredentialDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("not a non-link directory")
	}
	return verifyUnixOwnerMode(info, 0o700)
}

func verifyCredentialFile(_ string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular non-link file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return fmt.Errorf("credential file has unexpected links")
	}
	return verifyUnixOwnerMode(info, 0o600)
}

func verifyUnixOwnerMode(info os.FileInfo, mode os.FileMode) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("not owned by current user")
	}
	if info.Mode().Perm() != mode {
		return fmt.Errorf("permissions are %03o, require %03o", info.Mode().Perm(), mode)
	}
	return nil
}

func protectCredentialFile(path string) error      { return os.Chmod(path, 0o600) }
func protectCredentialDirectory(path string) error { return os.Chmod(path, 0o700) }

func syncCredentialDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func commitCredentialReplace(from, to string) namespaceMutation {
	if err := os.Rename(from, to); err != nil {
		return namespaceMutation{err: fmt.Errorf("replace MCP credential: %w", err)}
	}
	return namespaceMutation{changed: true, err: syncCredentialDirectory(filepathDir(to))}
}

func commitCredentialDelete(path string) namespaceMutation {
	if err := os.Remove(path); err != nil {
		return namespaceMutation{err: fmt.Errorf("delete MCP credential: %w", err)}
	}
	return namespaceMutation{changed: true, err: syncCredentialDirectory(filepathDir(path))}
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if os.IsPathSeparator(path[i]) {
			return path[:i]
		}
	}
	return "."
}
