package desktop

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"image/png"
	"net/http"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed assets/icon.png
var iconPNG []byte
var user = windows.NewLazySystemDLL("user32.dll")
var shell = windows.NewLazySystemDLL("shell32.dll")
var gdi = windows.NewLazySystemDLL("gdi32.dll")
var kernel = windows.NewLazySystemDLL("kernel32.dll")







func win(name string, args ...uintptr) uintptr { v, _, _ := user.NewProc(name).Call(args...); return v }

type point struct{ X, Y int32 }
type notifyIcon struct {
	Size                uint32
	Hwnd                uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Timeout             uint32
	Title               [64]uint16
	InfoFlags           uint32
	Guid                windows.GUID
	Balloon             uintptr
}

func appIcon() uintptr {
	im, e := png.Decode(bytes.NewReader(iconPNG))
	if e != nil {
		return 0
	}
	const n = 64
	// ICO image payload: bottom-up BGRA bitmap and transparency mask.
	b := make([]byte, 40+n*n*4+n*n/8)
	binary.LittleEndian.PutUint32(b, 40)
	binary.LittleEndian.PutUint32(b[4:], n)
	binary.LittleEndian.PutUint32(b[8:], n*2)
	binary.LittleEndian.PutUint16(b[12:], 1)
	binary.LittleEndian.PutUint16(b[14:], 32)
	bounds := im.Bounds()
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r, g, bl, a := im.At(bounds.Min.X+x*bounds.Dx()/n, bounds.Min.Y+(n-1-y)*bounds.Dy()/n).RGBA()
			i := 40 + (y*n+x)*4
			if a > 0 {
				b[i] = byte(bl * 255 / a)
				b[i+1] = byte(g * 255 / a)
				b[i+2] = byte(r * 255 / a)
			}
			b[i+3] = byte(a >> 8)
		}
	}
	return win("CreateIconFromResourceEx", uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 1, 0x30000, n, n, 0)
}
func chrome(w webview.WebView, url, token string, onReady func()) func() {
	hwnd := uintptr(w.Window())
	const bgRGB = 0x00231707
	brush, _, _ := gdi.NewProc("CreateSolidBrush").Call(bgRGB)
	if brush != 0 {
		user.NewProc("SetClassLongPtrW").Call(hwnd, uintptr(uint32(0xfffffff6)), brush)
	}
	icon := appIcon()
	win("SendMessageW", hwnd, 0x80, 0, icon)
	win("SendMessageW", hwnd, 0x80, 1, icon)
	style := win("GetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)))
	// Keep WS_CAPTION and WS_THICKFRAME: DWM uses these for native transitions.
	// WM_NCCALCSIZE below removes their visible non-client area instead.
	win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffff0)), style|0x00cf0000)
	n := notifyIcon{Hwnd: hwnd, ID: 1, Flags: 7, Callback: 0x8001, Icon: icon}
	n.Size = uint32(unsafe.Sizeof(n))
	copy(n.Tip[:], syscall.StringToUTF16("小涛 · 双击打开，右键菜单"))
	tray, _, _ := shell.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&n)))
	var readyOnce sync.Once
	startTime := time.Now()
	reveal := func() {
		readyOnce.Do(func() {
			elapsed := time.Since(startTime)
			const minDuration = 3900 * time.Millisecond
			if elapsed < minDuration {
				time.AfterFunc(minDuration-elapsed, func() {
					w.Dispatch(func() {
						if onReady != nil {
							onReady()
						}
						win("ShowWindow", hwnd, 9)
						win("SetForegroundWindow", hwnd)
					})
				})
				return
			}
			if onReady != nil {
				onReady()
			}
			win("ShowWindow", hwnd, 9)
			win("SetForegroundWindow", hwnd)
		})
	}
	time.AfterFunc(5500*time.Millisecond, func() {
		w.Dispatch(reveal)
	})
	show := func() { reveal(); win("ShowWindow", hwnd, 9); win("SetForegroundWindow", hwnd) }
	exit := func() {
		go func() {
			req, _ := http.NewRequest("POST", url+"/api/shutdown", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			r, e := http.DefaultClient.Do(req)
			if e == nil {
				r.Body.Close()
			}
		}()
	}
	var old uintptr
	taskbar := win("RegisterWindowMessageW", uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("TaskbarCreated"))))
	callback := syscall.NewCallback(func(h uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case 0x14:
			if brush != 0 {
				hdc := wp
				var r struct{ Left, Top, Right, Bottom int32 }
				user.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
				user.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
				return 1
			}
		case 0x83: // Let WebView cover the entire client area, including the old top frame.
			if wp != 0 {
				return 0
			}
		case 0x84: // Keep resizing available after removing the non-client frame.
			if win("IsZoomed", hwnd) == 0 {
				var r struct{ Left, Top, Right, Bottom int32 }
				win("GetWindowRect", hwnd, uintptr(unsafe.Pointer(&r)))
				x, y := int32(int16(lp&0xffff)), int32(int16((lp>>16)&0xffff))
				edge := int32(6)
				left, right, top, bottom := x < r.Left+edge, x >= r.Right-edge, y < r.Top+edge, y >= r.Bottom-edge
				switch {
				case top && left:
					return 13
				case top && right:
					return 14
				case bottom && left:
					return 16
				case bottom && right:
					return 17
				case left:
					return 10
				case right:
					return 11
				case top:
					return 12
				case bottom:
					return 15
				}
			}
		case 0x10:
			if tray != 0 {
				win("ShowWindow", hwnd, 0)
				return 0
			}
		case 0x8002:
			show()
			return 0
		case 0x8001:
			if lp == 0x203 {
				show()
			}
			if lp == 0x205 {
				menu := win("CreatePopupMenu")
				defer win("DestroyMenu", menu)
				for i, label := range []string{"打开小涛", "在浏览器打开", "退出小涛"} {
					win("AppendMenuW", menu, 0, uintptr(i+1), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(label))))
				}
				var p point
				win("GetCursorPos", uintptr(unsafe.Pointer(&p)))
				win("SetForegroundWindow", hwnd)
				// Anchor the bottom-right of the menu to the tray click, opening upward.
				// RETURNCMD | RIGHTBUTTON | RIGHTALIGN | BOTTOMALIGN | VERNEGANIMATION.
				id := win("TrackPopupMenu", menu, 0x100|0x2|0x8|0x20|0x1000, uintptr(p.X), uintptr(p.Y), 0, hwnd, 0)
				win("PostMessageW", hwnd, 0, 0, 0)
				switch id {
				case 1:
					show()
				case 2:
					Browser(url)
				case 3:
					exit()
				}
			}
			return 0
		}
		if msg == uint32(taskbar) {
			shell.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&n)))
		}
		return win("CallWindowProcW", old, h, uintptr(msg), wp, lp)
	})
	old = win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), callback)
	// Recalculate only after installing WM_NCCALCSIZE handling.
	win("SetWindowPos", hwnd, 0, 0, 0, 0, 0, 0x37)
	dark := uint32(1)
	windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	captionColor := uint32(bgRGB)
	windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(hwnd, 35, uintptr(unsafe.Pointer(&captionColor)), 4)
	transitionsDisabled := uint32(0)
	windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(hwnd, 3, uintptr(unsafe.Pointer(&transitionsDisabled)), 4)
	w.Bind("xtWindow", func(action string) {
		w.Dispatch(func() {
			edges := map[string]uintptr{"resize-w": 10, "resize-e": 11, "resize-n": 12, "resize-nw": 13, "resize-ne": 14, "resize-s": 15, "resize-sw": 16, "resize-se": 17}
			if edge, ok := edges[action]; ok {
				if win("IsZoomed", hwnd) == 0 {
					win("ReleaseCapture")
					win("SendMessageW", hwnd, 0x112, 0xf000+(edge-9), 0)
				}
				return
			}
			switch action {
			case "ready":
				reveal()
			case "minimize":
				win("PostMessageW", hwnd, 0x112, 0xf020, 0)
			case "maximize":
				if win("IsZoomed", hwnd) != 0 {
					win("PostMessageW", hwnd, 0x112, 0xf120, 0)
				} else {
					win("PostMessageW", hwnd, 0x112, 0xf030, 0)
				}
			case "close":
				win("SendMessageW", hwnd, 0x10, 0, 0)
			case "drag":
				win("ReleaseCapture")
				win("SendMessageW", hwnd, 0xa1, 2, 0)
			}
		})
	})
	return func() {
		shell.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&n)))
		win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), old)
		if brush != 0 {
			gdi.NewProc("DeleteObject").Call(brush)
		}
		if icon != 0 {
			win("DestroyIcon", icon)
		}
	}
}

func ShowExisting() bool {
	h := win("FindWindowW", 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("小涛"))))
	if h == 0 {
		return false
	}
	win("PostMessageW", h, 0x8002, 0, 0)
	return true
}
