package desktop

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"image"
	"image/draw"
	"image/jpeg"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed assets/frames.bin
var framesBIN []byte

func decodeFrame(data []byte, target []byte) (int32, int32) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	bounds := img.Bounds()
	w, h := int32(bounds.Dx()), int32(bounds.Dy())
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
	for y := int32(0); y < h; y++ {
		srcY := y
		dstY := h - 1 - y
		si := (srcY * w) * 4
		di := (dstY * w) * 4
		for x := int32(0); x < w; x++ {
			target[di] = rgba.Pix[si+2]
			target[di+1] = rgba.Pix[si+1]
			target[di+2] = rgba.Pix[si]
			target[di+3] = 0xFF
			si += 4
			di += 4
		}
	}
	return w, h
}

func showNativeSplash(width, height int32) func() {
	runtime.LockOSThread()

	r := bytes.NewReader(framesBIN)
	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil || count <= 0 {
		return func() {}
	}
	rawBuffers := make([][]byte, count)
	for i := int32(0); i < count; i++ {
		var sz int32
		if err := binary.Read(r, binary.LittleEndian, &sz); err != nil || sz <= 0 {
			break
		}
		buf := make([]byte, sz)
		if _, err := r.Read(buf); err != nil {
			break
		}
		rawBuffers[i] = buf
	}
	if len(rawBuffers) == 0 {
		return func() {}
	}

	const frameW, frameH = int32(1920), int32(1080)
	const frameBytes = int(frameW * frameH * 4)

	const poolSize = 4
	freeChan := make(chan []byte, poolSize)
	for i := 0; i < poolSize; i++ {
		freeChan <- make([]byte, frameBytes)
	}

	curBuf := <-freeChan
	decodeFrame(rawBuffers[0], curBuf)

	stopChan := make(chan struct{})
	frameChan := make(chan []byte, poolSize)

	go func() {
		defer close(frameChan)
		for i := 1; i < len(rawBuffers); i++ {
			select {
			case <-stopChan:
				return
			case buf := <-freeChan:
				decodeFrame(rawBuffers[i], buf)
				select {
				case <-stopChan:
					return
				case frameChan <- buf:
				}
			}
		}
	}()

	className, _ := windows.UTF16PtrFromString("XiaoTaoNativeSplash")
	hinstance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)

	const bgRGB = 0x00231707
	brush, _, _ := gdi.NewProc("CreateSolidBrush").Call(bgRGB)

	var parentHwnd uintptr

	wndProc := syscall.NewCallback(func(h uintptr, msg uint32, wp, lp uintptr) uintptr {
		switch msg {
		case 0x0014:
			return 1
		case 0x0005:
			roundWindowRegion(h, int32(lp&0xffff), int32((lp>>16)&0xffff))
			break
		case 0x0113:
			select {
			case nextBuf, ok := <-frameChan:
				if ok {
					if curBuf != nil {
						select {
						case freeChan <- curBuf:
						default:
						}
					}
					curBuf = nextBuf
					user.NewProc("InvalidateRect").Call(h, 0, 0)
				}
			default:
			}
			return 0
		case 0x000F:
			var ps struct {
				Hdc         uintptr
				FErase      int32
				RcPaint     struct{ Left, Top, Right, Bottom int32 }
				FRestore    int32
				FIncUpdate  int32
				RgbReserved [32]byte
			}
			hdc, _, _ := user.NewProc("BeginPaint").Call(h, uintptr(unsafe.Pointer(&ps)))

			if curBuf != nil {
				var bmi struct {
					BiSize          uint32
					BiWidth         int32
					BiHeight        int32
					BiPlanes        uint16
					BiBitCount      uint16
					BiCompression   uint32
					BiSizeImage     uint32
					BiXPelsPerMeter int32
					BiYPelsPerMeter int32
					BiClrUsed       uint32
					BiClrImportant  uint32
				}
				bmi.BiSize = 40
				bmi.BiWidth = frameW
				bmi.BiHeight = frameH
				bmi.BiPlanes = 1
				bmi.BiBitCount = 32

				var cr struct{ Left, Top, Right, Bottom int32 }
				user.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&cr)))
				cw := cr.Right - cr.Left
				ch := cr.Bottom - cr.Top

				gdi.NewProc("SetStretchBltMode").Call(hdc, 4)
				gdi.NewProc("SetBrushOrgEx").Call(hdc, 0, 0, 0)
				gdi.NewProc("StretchDIBits").Call(
					hdc,
					0, 0, uintptr(cw), uintptr(ch),
					0, 0, uintptr(frameW), uintptr(frameH),
					uintptr(unsafe.Pointer(&curBuf[0])),
					uintptr(unsafe.Pointer(&bmi)),
					0,
					0x00CC0020,
				)
			}

			user.NewProc("EndPaint").Call(h, uintptr(unsafe.Pointer(&ps)))
			return 0
		case 0x0010:
			user.NewProc("DestroyWindow").Call(h)
			return 0
		}
		r, _, _ := user.NewProc("DefWindowProcW").Call(h, uintptr(msg), wp, lp)
		return r
	})

	var wc struct {
		CbSize        uint32
		Style         uint32
		LpfnWndProc   uintptr
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     uintptr
		HIcon         uintptr
		HCursor       uintptr
		HbrBackground uintptr
		LpszMenuName  uintptr
		LpszClassName uintptr
		HIconSm       uintptr
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = wndProc
	wc.HInstance = hinstance
	wc.HbrBackground = brush
	wc.LpszClassName = uintptr(unsafe.Pointer(className))
	user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))

	sw, _, _ := user.NewProc("GetSystemMetrics").Call(0)
	sh, _, _ := user.NewProc("GetSystemMetrics").Call(1)
	x := (int32(sw) - width) / 2
	y := (int32(sh) - height) / 2

	parentHwnd = win("CreateWindowExW",
		0x00080088,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0x80000000,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		0, 0, hinstance, 0,
	)

	user.NewProc("SetLayeredWindowAttributes").Call(parentHwnd, 0, 255, 2)
	roundNativeWindow(parentHwnd)
	roundWindowRegion(parentHwnd, width, height)
	user.NewProc("ShowWindow").Call(parentHwnd, 5)
	user.NewProc("UpdateWindow").Call(parentHwnd)
	user.NewProc("SetTimer").Call(parentHwnd, 1, 42, 0)

	var closeOnce sync.Once
	return func() {
		closeOnce.Do(func() {
			close(stopChan)
			if parentHwnd != 0 {
				user.NewProc("KillTimer").Call(parentHwnd, 1)
				user.NewProc("DestroyWindow").Call(parentHwnd)
				parentHwnd = 0
			}
			if brush != 0 {
				gdi.NewProc("DeleteObject").Call(brush)
			}
			curBuf = nil
			runtime.GC()
		})
	}
}
