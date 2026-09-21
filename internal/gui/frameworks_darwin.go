//go:build desktop && darwin

package gui

// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"

// Current Wails native file dialogs reference UTType from this framework.
