//go:build desktop && !darwin

package gui

import (
	"github.com/PureStudyer/SEU-SC-Bridge/frontend"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"github.com/energye/systray"

	goruntime "runtime"
)

func runTray(p platform.Paths) error { return run(p, true) }

func startTrayBeforeRun(func()) {}

func startTrayAfterRun(start func()) { start() }

func configureTrayAppearance() {
	if goruntime.GOOS == "windows" {
		systray.SetIcon(frontend.IconICO)
	} else {
		systray.SetIcon(frontend.TrayPNG)
	}
	systray.SetTitle("SEU SC Bridge")
	systray.SetTooltip("SEU SC Bridge · 本地 SSH")
}

func finishTrayMenu() {}
