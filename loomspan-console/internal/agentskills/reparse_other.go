//go:build !windows

package agentskills

import "os"

func isReparsePoint(os.FileInfo) bool { return false }
