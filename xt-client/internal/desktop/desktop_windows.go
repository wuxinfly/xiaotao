package desktop

import (
	"bytes"
	"fmt"
	webview "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	windowWidth  = 1280
	windowHeight = 800
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
		if arg == "--window" || arg == "--hud-window" {
			hidden = false
		}
	}
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: hidden, CreationFlags: 0x00000008}
	return c.Start()
}

func HUDWindowTitle(displayName string) string {
	return displayName + " 请求浮窗"
}

func WindowExists(title string) bool {
	p, _ := windows.UTF16PtrFromString(title)
	return win("FindWindowW", 0, uintptr(unsafe.Pointer(p))) != 0
}

func CloseWindow(title string) bool {
	p, _ := windows.UTF16PtrFromString(title)
	hwnd := win("FindWindowW", 0, uintptr(unsafe.Pointer(p)))
	if hwnd == 0 {
		return false
	}
	win("PostMessageW", hwnd, 0x10, 0, 0)
	return true
}

func HUDWindow(url, data, token, displayName string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		return fmt.Errorf("WebView2 unavailable: %w", err)
	}
	title := HUDWindowTitle(displayName)
	w := webview.NewWithOptions(webview.WebViewOptions{
		DataPath:  filepath.Join(data, "webview"),
		AutoFocus: true,
		WindowOptions: webview.WindowOptions{
			Title:  title,
			Width:  340,
			Height: 260,
		},
	})
	if w == nil {
		return fmt.Errorf("cannot create WebView2 window")
	}
	hwnd := uintptr(w.Window())
	style := win("GetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)))
	// Borderless tool window: the WebView header owns dragging, and the
	// fixed 340x260 surface stays lightweight like a status overlay.
	win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)), style&^uintptr(0x00c00000|0x00040000|0x00010000|0x00080000))
	roundNativeWindow(hwnd)
	// Recalculate the client area after removing the frame, then resize the
	// WebView controller to match it (otherwise the old frame leaves white strips).
	win("SetWindowPos", hwnd, uintptr(^uint32(0)), 0, 0, 340, 260, 0x0022)
	win("SendMessageW", hwnd, 0x0005, 0, 260<<16|340) // WM_SIZE refreshes WebView bounds.
	defer w.Destroy()
	if err := w.Bind("xtHud", func(action string) error {
		if action == "close" {
			w.Dispatch(func() { w.Terminate() })
		}
		if action == "drag" {
			win("ReleaseCapture")
			win("SendMessageW", hwnd, 0xa1, 2, 0)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("bind HUD actions: %w", err)
	}
	w.Navigate(url + "/hud.html")
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
				w.Dispatch(func() { w.Terminate() })
				return
			}
		}
	}()
	w.Run()
	disableHUD(url, token)
	return nil
}

func disableHUD(url, token string) {
	request, err := http.NewRequest("POST", url+"/api/request-hud", bytes.NewBufferString(`{"enabled":false}`))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err == nil {
		response.Body.Close()
	}
}
func Browser(url string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

func Alert(titleText, message string) {
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString(titleText)
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
func Window(url, data, token, title string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := webviewloader.GetInstalledVersion(); err != nil {
		return fmt.Errorf("WebView2 unavailable: %w", err)
	}

	closeSplash := showNativeSplash(windowWidth, windowHeight)
	defer closeSplash()

	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: filepath.Join(data, "webview"), AutoFocus: true, WindowOptions: webview.WindowOptions{Title: title, Width: windowWidth, Height: windowHeight, Center: true}})
	if w == nil {
		return fmt.Errorf("cannot create WebView2 window")
	}
	roundNativeWindow(uintptr(w.Window()))
	defer w.Destroy()
	if err := w.Bind("xtOpenDirectory", func(kind string) error {
		var directory string
		switch kind {
		case "install":
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("获取安装目录失败: %w", err)
			}
			directory = filepath.Dir(executable)
		case "data":
			directory = data
		default:
			return fmt.Errorf("未知目录操作")
		}
		absolute, err := filepath.Abs(directory)
		if err != nil {
			return fmt.Errorf("获取目录路径失败: %w", err)
		}
		path, err := windows.UTF16PtrFromString(absolute)
		if err != nil {
			return err
		}
		if err := windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), path, nil, nil, 1); err != nil {
			return fmt.Errorf("打开目录失败: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("bind directory actions: %w", err)
	}
	if err := w.Bind("xtPickFolder", func() (string, error) {
		return PickFolder("选择小涛要工作的项目文件夹")
	}); err != nil {
		return fmt.Errorf("bind folder picker: %w", err)
	}
	// Start in the document-init hook. The shipped go-webview2 RPC binding
	// invokes Go on the WebView2 message thread, which can lose the modal dialog
	// owner and tear down the window when the picker closes.
	w.Init(`window.xtPickFolderOnly = () => window.xtWindow?.("pick-folder");`)
	cleanup := chrome(w, url, token, title, closeSplash)
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
