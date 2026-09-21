//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func PrepareGUI() {
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	var ids [2]uint32
	n, _, _ := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&ids[0])), 2)
	if n == 1 {
		kernel.NewProc("FreeConsole").Call()
	}
}
