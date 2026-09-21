//go:build desktop

package platform

import (
	"os"
	"os/exec"
)

func StartTray() {
	if os.Getenv("SEUSC_NO_TRAY") == "1" {
		return
	}
	exe, e := os.Executable()
	if e != nil {
		return
	}
	cmd := exec.Command(exe, "tray")
	Detached(cmd)
	if cmd.Start() == nil {
		_ = cmd.Process.Release()
	}
}
