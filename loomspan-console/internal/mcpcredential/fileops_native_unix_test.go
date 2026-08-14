//go:build linux || darwin

package mcpcredential

import "testing"

func TestNativeUnixCredentialCommitPrimitives(t *testing.T) {
	exerciseNativeCredentialOperations(t)
}
