package desktop

import (
	"runtime"
	"testing"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func TestStartupWebViewStaysHidden(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	status, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(status) < 0 {
		t.Fatalf("CoInitializeEx: %x", status)
	}
	defer ole.NewProc("CoUninitialize").Call()
	finish, err := hideWebViewDuringCreation()
	if err != nil {
		t.Fatal(err)
	}
	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: t.TempDir()})
	finish()
	if w == nil {
		t.Fatal("WebView creation failed")
	}
	hwnd := uintptr(w.Window())
	defer win("DestroyWindow", hwnd)
	if win("IsWindowVisible", hwnd) != 0 {
		t.Error("startup WebView was shown before ready")
	}
	if win("GetForegroundWindow") == hwnd {
		t.Error("startup WebView stole foreground")
	}
	// Removing the temporary guard must allow the ready handoff to show it.
	win("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
	if win("IsWindowVisible", hwnd) == 0 {
		t.Error("ready WebView could not be shown")
	}
}
