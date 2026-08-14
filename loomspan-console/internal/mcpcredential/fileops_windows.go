//go:build windows

package mcpcredential

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/windowsacl"
	"golang.org/x/sys/windows"
)

func verifyCredentialDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	return verifyWindowsCredentialProtection(path)
}

func verifyCredentialFile(path string, info os.FileInfo) error {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular non-reparse file")
	}
	return verifyWindowsCredentialProtection(path)
}

func protectCredentialFile(path string) error {
	return protectWindowsCredentialPath(path, false)
}

func protectCredentialDirectory(path string) error {
	return protectWindowsCredentialPath(path, true)
}

func protectWindowsCredentialPath(path string, directory bool) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.String() + "D:P(A;" + inheritance + ";FA;;;" + user.String() + ")(A;" + inheritance + ";FA;;;SY)(A;" + inheritance + ";FA;;;BA)")
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, dacl, nil)
}

func currentUserSID() (*windows.SID, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid.Copy()
}

func verifyWindowsCredentialProtection(path string) error {
	user, err := currentUserSID()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return fmt.Errorf("cannot inspect owner and DACL")
	}
	owner, _, err := sd.Owner()
	if err != nil || !owner.Equals(user) {
		return fmt.Errorf("not owned by current user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("DACL is not protected")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("cannot inspect DACL")
	}
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	administrators, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if !windowsacl.GrantsOnly(dacl, user, system, administrators) {
		return fmt.Errorf("DACL grants unexpected principals")
	}
	return nil
}

func flushCredentialFile(file *os.File) error {
	return windows.FlushFileBuffers(windows.Handle(file.Fd()))
}

func commitCredentialEnable(from, to string) namespaceMutation {
	return moveCredential(from, to, false)
}
func commitCredentialReplace(from, to string) namespaceMutation {
	return moveCredential(from, to, true)
}

func moveCredential(from, to string, replace bool) namespaceMutation {
	fromUTF16, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return namespaceMutation{err: err}
	}
	toUTF16, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return namespaceMutation{err: err}
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	if err := windows.MoveFileEx(fromUTF16, toUTF16, flags); err != nil {
		// A write-through move can report a durability failure after the
		// namespace changed. Conservatively force canonical re-inspection.
		return namespaceMutation{changed: true, err: fmt.Errorf("commit MCP credential: %w", err)}
	}
	return namespaceMutation{changed: true}
}

func commitCredentialDelete(path string) namespaceMutation {
	pathUTF16, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return namespaceMutation{err: err}
	}
	if err := windows.DeleteFile(pathUTF16); err != nil {
		return namespaceMutation{changed: true, err: fmt.Errorf("delete MCP credential: %w", err)}
	}
	return namespaceMutation{changed: true}
}
