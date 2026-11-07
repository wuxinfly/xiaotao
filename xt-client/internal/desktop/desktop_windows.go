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
	show := uint16(windows.SW_SHOWNORMAL)
	for _, arg := range args {
		if arg == "--window" || arg == "--hud-window" {
			hidden = false
		}
		if arg == "--hud-window" {
			show = windows.SW_SHOWNOACTIVATE
		}
	}
	if !hidden {
		// Explicit visibility prevents GUI children from inheriting a hidden
		// service launch's startup state (including WebView2 subprocesses).
		application, err := windows.UTF16PtrFromString(exe)
		if err != nil {
			return err
		}
		command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{exe}, args...)))
		if err != nil {
			return err
		}
		startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: show}
		var process windows.ProcessInformation
		if err := windows.CreateProcess(application, command, nil, nil, false, windows.DETACHED_PROCESS, nil, nil, &startup, &process); err != nil {
			return err
		}
		windows.CloseHandle(process.Thread)
		windows.CloseHandle(process.Process)
		return nil
	}
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: hidden, CreationFlags: 0x00000008}
	return c.Start()
}

func HUDWindowTitle(displayName string) string {
	return displayName + " 请求浮窗"
}

func WindowExists(title string) bool {
	p, _ := windows.UTF16PtrFromString(title)
	class := windows.StringToUTF16Ptr("webview")
	return win("FindWindowW", uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(p))) != 0
}

func CloseWindow(title string) bool {
	p, _ := windows.UTF16PtrFromString(title)
	class := windows.StringToUTF16Ptr("webview")
	hwnd := win("FindWindowW", uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(p)))
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
			Width:  uint(hudExpandedWidth),
			Height: uint(hudExpandedHeight),
		},
	})
	if w == nil {
		return fmt.Errorf("cannot create WebView2 window")
	}
	hwnd := uintptr(w.Window())
	cleanupHUD := configureHUDWindow(hwnd)
	defer cleanupHUD()
	defer w.Destroy()
	dock := newHUDDock(hwnd, func(script string) {
		w.Dispatch(func() { w.Eval(script) })
	})
	defer dock.close()
	if err := w.Bind("xtHud", func(action string) error {
		if action == "close" {
			w.Dispatch(func() { w.Terminate() })
		}
		if action == "drag" {
			win("ReleaseCapture")
			win("SendMessageW", hwnd, 0xa1, 2, 0)
		}
		if action == "ready" {
			dock.activate()
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

func configureHUDWindow(hwnd uintptr) func() {
	// Changing taskbar eligibility requires hiding the already-created window.
	win("ShowWindow", hwnd, 0)
	exStyle := win("GetWindowLongPtrW", hwnd, uintptr(uint32(0xffffffec)))
	win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xffffffec)), (exStyle&^uintptr(0x00040000))|0x00000080) // TOOLWINDOW, no APPWINDOW
	style := win("GetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)))
	win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)), style&^uintptr(0x00c00000|0x00040000|0x00010000|0x00080000))
	resizeChild := func() {
		var client rect
		win("GetClientRect", hwnd, uintptr(unsafe.Pointer(&client)))
		child := win("GetWindow", hwnd, 5)
		if child != 0 {
			win("SetWindowPos", child, 0, 0, 0, uintptr(client.Right), uintptr(client.Bottom), 0x0014)
		}
	}
	var old uintptr
	callback := syscall.NewCallback(func(h uintptr, msg uint32, wp, lp uintptr) uintptr {
		if msg == 0x83 && wp != 0 {
			return 0
		} // WM_NCCALCSIZE
		result := win("CallWindowProcW", old, h, uintptr(msg), wp, lp)
		if msg == 0x5 {
			resizeChild()
		} // Controller first, child second.
		if msg == 0x02e0 {
			dpi := uint32(wp & 0xffff)
			win("SetWindowPos", h, 0, 0, 0, uintptr(hudPixels(hudExpandedWidth, dpi)), uintptr(hudPixels(hudExpandedHeight, dpi)), 0x0016)
		}
		return result
	})
	old = win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), callback)
	dpi := win("GetDpiForWindow", hwnd)
	if dpi == 0 {
		dpi = 96
	}
	width, height := hudPixels(hudExpandedWidth, uint32(dpi)), hudPixels(hudExpandedHeight, uint32(dpi))
	// HWND_TOPMOST must be pointer-sized -1 on Windows amd64.
	win("SetWindowPos", hwnd, ^uintptr(0), 0, 0, uintptr(width), uintptr(height), 0x0032)
	resizeChild()
	roundNativeWindow(hwnd)
	win("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
	return func() { win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), old) }
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

	closeSplash, splashForeground, attachSplash := showNativeSplash(windowWidth, windowHeight)
	defer closeSplash()

	// WebView2 reads this at environment creation; match the native/page background.
	previousBackground, hadBackground := os.LookupEnv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR")
	if err := os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF071723"); err != nil {
		return err
	}
	defer func() {
		if hadBackground {
			os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", previousBackground)
		} else {
			os.Unsetenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR")
		}
	}()
	finishCreation, err := guardWebViewDuringCreation(attachSplash)
	if err != nil {
		return err
	}
	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: filepath.Join(data, "webview"), AutoFocus: true, WindowOptions: webview.WindowOptions{Title: title, Width: windowWidth, Height: windowHeight, Center: true}})
	finishCreation()
	if w == nil {
		return fmt.Errorf("cannot create WebView2 window")
	}
	attachSplash(uintptr(w.Window()))
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
	cleanup := chrome(w, url, token, title, closeSplash, splashForeground)
	defer cleanup()
	resizeStartupCover(uintptr(w.Window()))
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
