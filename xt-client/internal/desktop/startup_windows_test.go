package desktop

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
	"unsafe"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func TestStartupWebViewLoadsFrontendBehindSplash(t *testing.T) {
	testStartupWebView(t, false)
}

func TestStartupHandoffKeepsMinimizedWindow(t *testing.T) {
	testStartupWebView(t, true)
}

func testStartupWebView(t *testing.T, minimize bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	status, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2)
	if int32(status) < 0 {
		t.Fatalf("CoInitializeEx: %x", status)
	}
	defer ole.NewProc("CoUninitialize").Call()
	closeSplash, _, attachSplash := showNativeSplash(windowWidth, windowHeight)
	defer closeSplash()
	finish, err := guardWebViewDuringCreation(attachSplash)
	if err != nil {
		t.Fatal(err)
	}
	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: t.TempDir(), WindowOptions: webview.WindowOptions{Width: windowWidth, Height: windowHeight, Center: true}})
	finish()
	if w == nil {
		t.Fatal("WebView creation failed")
	}
	hwnd := uintptr(w.Window())
	if win("GetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)))&0x00c00000 != 0 {
		t.Error("startup window must be borderless before chrome initialization")
	}
	attachSplash(hwnd)
	cover := win("FindWindowExW", hwnd, 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("XiaoTaoNativeSplash"))), 0)
	if cover == 0 || win("GetParent", cover) != hwnd {
		t.Fatal("startup cover must belong to the main window surface")
	}
	defer win("DestroyWindow", hwnd)
	if win("IsWindowVisible", hwnd) == 0 {
		t.Error("startup WebView must stay visible to render behind splash")
	}
	if win("GetForegroundWindow") == hwnd {
		t.Error("startup WebView stole foreground")
	}
	server := httptest.NewServer(http.FileServer(http.Dir("../app/web")))
	defer server.Close()
	ready := false
	cleanup := chrome(w, server.URL, "", "小涛启动回归验证", func() {
		closeSplash()
		ready = true
		w.Terminate()
	}, func() bool { return false })
	defer cleanup()
	w.Navigate(server.URL)
	if minimize {
		win("ShowWindow", hwnd, 6)
	}
	timeout := time.AfterFunc(7*time.Second, func() { w.Dispatch(w.Terminate) })
	w.Run()
	timeout.Stop()
	if !ready {
		t.Error("real frontend did not complete startup handoff before fallback")
	}
	if minimize && win("IsIconic", hwnd) == 0 {
		t.Error("startup handoff restored a window minimized by the user")
	}
	// Closing the owned cover must leave the rendered main window visible.
	closeSplash()
	win("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
	if win("IsWindowVisible", hwnd) == 0 {
		t.Error("ready WebView could not be shown")
	}
}
