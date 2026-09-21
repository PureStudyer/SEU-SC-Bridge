//go:build windows

package platform

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os/exec"
	"syscall"
)

func Detached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
}
func Protect(path string) error {
	token := windows.GetCurrentProcessToken()
	u, e := token.GetTokenUser()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + u.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if e != nil {
		return e
	}
	dacl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
func AutoStart(exe string, enabled bool) error {
	k, _, e := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	if !enabled {
		e = k.DeleteValue("SEUSC")
		if e == registry.ErrNotExist {
			return nil
		}
		return e
	}
	return k.SetStringValue("SEUSC", fmt.Sprintf("%q agent", exe))
}
