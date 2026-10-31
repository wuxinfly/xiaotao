package desktop

import (
	"fmt"
	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func Lock(name string) (func(), error) {
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateMutex(nil, false, p)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
func Start(exe string, args ...string) error {
	c := exec.Command(exe, args...)
	hidden := true
	for _, arg := range args {
		if arg == "--window" {
			hidden = false
		}
	}
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: hidden, CreationFlags: 0x00000008}
	return c.Start()
}
func Browser(url string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

func Alert(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("小涛")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
func Window(url, data, token string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		return fmt.Errorf("WebView2 unavailable: %w", err)
	}

	closeSplash := playNativeSplash()
	defer closeSplash()

	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: filepath.Join(data, "webview"), AutoFocus: true, WindowOptions: webview.WindowOptions{Title: "小涛", Width: 1280, Height: 720, Center: true}})
	if w == nil {
		return fmt.Errorf("cannot create WebView2 window")
	}
	defer w.Destroy()
	cleanup := chrome(w, url, token, closeSplash)
	defer cleanup()
	w.Navigate(url)
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		client := http.Client{Timeout: time.Second}
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				response, err := client.Get(url + "/health")
				if err == nil {
					response.Body.Close()
					continue
				}
				select {
				case <-done:
					return
				default:
				}
				w.Dispatch(func() { w.Terminate() })
				return
			}
		}
	}()
	w.Run()
	return nil
}
