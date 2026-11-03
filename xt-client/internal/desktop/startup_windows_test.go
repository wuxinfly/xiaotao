package desktop

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

func TestStartupWebViewLoadsFrontendBehindSplash(t *testing.T) {
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
	timeout := time.AfterFunc(7*time.Second, func() { w.Dispatch(w.Terminate) })
	w.Run()
	timeout.Stop()
	if !ready {
		t.Error("real frontend did not complete startup handoff before fallback")
	}
	// Closing the owned cover must leave the rendered main window visible.
	closeSplash()
	win("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
	if win("IsWindowVisible", hwnd) == 0 {
		t.Error("ready WebView could not be shown")
	}
}
