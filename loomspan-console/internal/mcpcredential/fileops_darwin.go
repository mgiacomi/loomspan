//go:build darwin

package mcpcredential

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func flushCredentialFile(file *os.File) error {
	_, err := unix.FcntlInt(file.Fd(), unix.F_FULLFSYNC, 0)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return file.Sync()
	}
	return err
}

func commitCredentialEnable(from, to string) namespaceMutation {
	if err := unix.RenameatxNp(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCL); err != nil {
		return namespaceMutation{err: fmt.Errorf("enable MCP credential exclusively: %w", err)}
	}
	return namespaceMutation{changed: true, err: syncCredentialDirectory(filepathDir(to))}
}
