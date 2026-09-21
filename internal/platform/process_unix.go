//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"syscall"
)

func Detached(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func Protect(path string) error {
	info, e := os.Stat(path)
	if e != nil {
		return e
	}
	if info.IsDir() {
		return os.Chmod(path, 0700)
	}
	return os.Chmod(path, 0600)
}
