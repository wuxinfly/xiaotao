package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var dwm = windows.NewLazySystemDLL("dwmapi.dll")

func roundNativeWindow(hwnd uintptr) {
	preference := int32(3) // DWMWCP_ROUNDSMALL
	dwm.NewProc("DwmSetWindowAttribute").Call(hwnd, 33, uintptr(unsafe.Pointer(&preference)), 4)
}

func roundWindowRegion(hwnd uintptr, width, height int32) {
	if hwnd == 0 || width <= 0 || height <= 0 {
		return
	}
	const cornerDiameter = 16
	region, _, _ := gdi.NewProc("CreateRoundRectRgn").Call(
		0, 0, uintptr(width+1), uintptr(height+1),
		cornerDiameter, cornerDiameter,
	)
	if region == 0 {
		return
	}
	if win("SetWindowRgn", hwnd, region, 1) == 0 {
		gdi.NewProc("DeleteObject").Call(region)
	}
}
