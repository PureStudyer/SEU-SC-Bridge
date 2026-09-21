//go:build desktop && darwin

package gui

// Wails native file dialogs reference UTType on current macOS SDKs.
// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
