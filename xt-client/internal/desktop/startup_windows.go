package desktop

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The WebView library calls ShowWindow before Embed and has no hidden option.
// Intercept only its window on this UI thread until the constructor returns.
func hideWebViewDuringCreation() (func(), error) {
	var hwnd, original uintptr
	proc := syscall.NewCallback(func(h uintptr, msg uint32, wp uintptr, lp unsafe.Pointer) uintptr {
		if msg == 0x0046 { // WM_WINDOWPOSCHANGING
			pos := (*struct {
				Hwnd, InsertAfter   uintptr
				X, Y, Width, Height int32
				Flags               uint32
			})(lp)
			pos.Flags = (pos.Flags &^ 0x0040) | 0x0010 // remove SHOWWINDOW; NOACTIVATE
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
