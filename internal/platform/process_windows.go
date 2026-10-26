//go:build windows

package platform

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func processStartTime() (uint64, error) {
	h, err := windows.GetCurrentProcess()
	if err != nil {
		return 0, err
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}

func RecoverStaleAgent(p Paths, expectedExe string) (bool, error) {
	info, err := readAgentPID(agentPIDPath(p))
	if err != nil {
		return false, nil
	}
	if info.PID == os.Getpid() || !strings.EqualFold(filepath.Clean(info.Executable), filepath.Clean(expectedExe)) {
		return false, nil
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(info.PID))
	if err != nil {
		return false, nil
	}
	defer windows.CloseHandle(h)
	image := make([]uint16, 32768)
	imageSize := uint32(len(image))
	if err = windows.QueryFullProcessImageName(h, 0, &image[0], &imageSize); err != nil {
		return false, nil
	}
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(image[:imageSize])), filepath.Clean(expectedExe)) {
		return false, nil
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return false, nil
	}
	started := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if started != info.Started {
		return false, nil
	}
	if err = windows.TerminateProcess(h, 1); err != nil {
		return false, err
	}
	var wait uint32
	wait, err = windows.WaitForSingleObject(h, 5000)
	if err != nil {
		return false, err
	}
	if wait != windows.WAIT_OBJECT_0 {
		return false, fmt.Errorf("timed out while waiting for the unresponsive agent to exit")
	}
	ClearAgentPID(agentPIDPath(p), info.PID)
	return true, nil
}

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
