//go:build desktop && darwin

package gui

import (
	"github.com/PureStudyer/SEU-SC-Bridge/frontend"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"github.com/energye/systray"
)

func runTray(p platform.Paths) error { return run(p, true) }

// systray creates an AppKit NSStatusItem. Wails invokes OnStartup on a
// goroutine, so macOS must start systray before wails.Run while the Go entry
// goroutine is still locked to the process main thread.
func startTrayBeforeRun(start func()) { start() }

func startTrayAfterRun(func()) {}

func configureTrayAppearance() {
	systray.SetTemplateIcon(frontend.TrayPNG, frontend.TrayPNG)
	systray.SetTitle("")
	systray.SetTooltip("SEU SC Bridge · 本地 SSH")
}

// energye/systray builds the native menu but does not attach it to the macOS
// status item until CreateMenu is called.
func finishTrayMenu() { systray.CreateMenu() }
