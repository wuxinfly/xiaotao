package desktop

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The WebView library shows its window before Embed. Keep it visible so that
// Chromium can render, covered by an owned splash, and prevent activation.
// The guard applies only to this UI thread until the constructor returns.
func guardWebViewDuringCreation(attachSplash func(uintptr)) (func(), error) {
	var hwnd, original uintptr
	proc := syscall.NewCallback(func(h uintptr, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
		if msg == 0x0083 && wp != 0 { // WM_NCCALCSIZE: no temporary system title bar.
			return 0
		}
		if msg == 0x0001 { // WM_CREATE: parent is valid before the library first shows it.
			setStartupTransitions(h, true)
			style := win("GetWindowLongPtrW", h, uintptr(uint32(0xfffffff0)))
			win("SetWindowLongPtrW", h, uintptr(uint32(0xfffffff0)), style&^uintptr(0x00c00000))
			result := win("CallWindowProcW", original, h, uintptr(msg), wp, uintptr(lp))
			if attachSplash != nil {
				attachSplash(h)
			}
			return result
		}
		if msg == 0x0005 {
			result := win("CallWindowProcW", original, h, uintptr(msg), wp, uintptr(lp))
			resizeStartupCover(h)
			return result
		}
		if msg == 0x0014 { // WM_ERASEBKGND, before chrome installs its background.
			var bounds rect
			win("GetClientRect", h, uintptr(unsafe.Pointer(&bounds)))
			brush, _, _ := gdi.NewProc("GetStockObject").Call(18) // DC_BRUSH
			previous, _, _ := gdi.NewProc("SetDCBrushColor").Call(wp, 0x00231707)
			user.NewProc("FillRect").Call(wp, uintptr(unsafe.Pointer(&bounds)), brush)
			gdi.NewProc("SetDCBrushColor").Call(wp, previous)
			return 1
		}
		if msg == 0x0046 { // WM_WINDOWPOSCHANGING
			pos := (*struct {
				Hwnd, InsertAfter   uintptr
				X, Y, Width, Height int32
				Flags               uint32
			})(lp)
			// Recalculate cached non-client metrics before the first visible frame.
			pos.Flags |= 0x0030 // FRAMECHANGED | NOACTIVATE; preserve SHOWWINDOW.
		}
		if msg == 0x0047 { // WM_WINDOWPOSCHANGED: synchronize cover before painting.
			result := win("CallWindowProcW", original, h, uintptr(msg), wp, uintptr(lp))
			resizeStartupCover(h)
			return result
		}
		return win("CallWindowProcW", original, h, uintptr(msg), wp, uintptr(lp))
	})
	hookProc := syscall.NewCallback(func(code int32, wp uintptr, lp unsafe.Pointer) uintptr {
		if code == 3 && hwnd == 0 { // HCBT_CREATEWND
			// GetClassName is not available yet for the first created window.
			created := (*struct {
				Create *struct {
					Param, Instance, Menu, Parent uintptr
					Height, Width, Y, X, Style    int32
					Name                          *uint16
					Class                         *uint16
					ExStyle                       uint32
				}
				InsertAfter uintptr
			})(lp)
			class := created.Create.Class
			if uintptr(unsafe.Pointer(class)) > 0xffff && windows.UTF16PtrToString(class) == "webview" {
				created.Create.Style &^= int32(0x00c00000) // WS_CAPTION, before first display.
				hwnd = wp
				original = win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), proc)
			}
		}
		if code == 5 && wp == hwnd && hwnd != 0 { // HCBT_ACTIVATE
			return 1
		}
		return win("CallNextHookEx", 0, uintptr(code), wp, uintptr(lp))
	})
	thread, _, _ := kernel.NewProc("GetCurrentThreadId").Call()
	hook := win("SetWindowsHookExW", 5, hookProc, 0, thread) // WH_CBT, thread-local
	if hook == 0 {
		return nil, fmt.Errorf("cannot guard startup window visibility")
	}
	return func() {
		win("UnhookWindowsHookEx", hook)
		if hwnd != 0 && original != 0 {
			win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), original)
		}
	}, nil
}
