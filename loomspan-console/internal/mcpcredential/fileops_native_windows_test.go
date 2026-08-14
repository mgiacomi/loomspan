//go:build windows

package mcpcredential

import "testing"

func TestNativeWindowsCredentialCommitPrimitives(t *testing.T) {
	exerciseNativeCredentialOperations(t)
}
