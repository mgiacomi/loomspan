//go:build windows

package agentskills

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func isReparsePoint(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
