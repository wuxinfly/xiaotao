package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type monitorInfo struct {
	Size    uint32
	Monitor windows.Rect
	Work    windows.Rect
	Flags   uint32
}

func resizeMaximizedChild(hwnd uintptr, info monitorInfo) {
	var window windows.Rect
	win("GetWindowRect", hwnd, uintptr(unsafe.Pointer(&window)))
	x, y := info.Work.Left-window.Left, info.Work.Top-window.Top
	width, height := info.Work.Right-info.Work.Left, info.Work.Bottom-info.Work.Top
	child := win("GetWindow", hwnd, 5) // GW_CHILD
	if child != 0 {
		win("SetWindowPos", child, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x0014)
	}
}
