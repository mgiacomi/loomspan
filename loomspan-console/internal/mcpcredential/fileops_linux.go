//go:build linux

package mcpcredential

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func flushCredentialFile(file *os.File) error { return file.Sync() }

func commitCredentialEnable(from, to string) namespaceMutation {
	return commitCredentialEnableLinux(from, to, unix.Renameat2)
}

func commitCredentialEnableLinux(from, to string, renameNoReplace func(int, string, int, string, uint) error) namespaceMutation {
	err := renameNoReplace(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		if err = os.Link(from, to); err == nil {
			if removeErr := os.Remove(from); removeErr != nil {
				return namespaceMutation{changed: true, err: fmt.Errorf("remove linked MCP credential temporary file: %w", removeErr)}
			}
		}
	}
	if err != nil {
		return namespaceMutation{err: fmt.Errorf("enable MCP credential exclusively: %w", err)}
	}
	return namespaceMutation{changed: true, err: syncCredentialDirectory(filepathDir(to))}
}
