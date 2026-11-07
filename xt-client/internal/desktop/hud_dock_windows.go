package desktop

import (
	"syscall"
	"time"
	"unsafe"
)

const (
	hudExpandedWidth  = 260
	hudExpandedHeight = 170
	hudCollapsedWidth = 32
	hudCollapsedEdge  = 6
	hudGap            = 14
	hudPollInterval   = 80
)

type hudDock struct {
	hwnd        uintptr
	old         uintptr
	edge        int
	collapsed   bool
	dragging    bool
	arranging   bool
	hover       bool
	collapseAt  time.Time
	activateAt  time.Time
	initialized bool
	dpi         uint32
	callback    uintptr
	eval        func(string)
}

type windowPos struct {
	Hwnd, InsertAfter   uintptr
	X, Y, Width, Height int32
	Flags               uint32
}

func lpToPointer(lp uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&lp))
}

func newHUDDock(hwnd uintptr, eval func(string)) *hudDock {
	dock := &hudDock{hwnd: hwnd, edge: 1, eval: eval}
	dock.dpi = hudDPI(hwnd)
	dock.callback = syscall.NewCallback(dock.handleMessage)
	dock.old = win("SetWindowLongPtrW", hwnd, uintptr(uint32(0xfffffffc)), dock.callback)
	dock.setPinned(true)
	dock.syncInitialBounds()
	dock.activateAt = time.Now().Add(8 * time.Second)
	dock.watchPointer()
	return dock
}

func (d *hudDock) activate() {
	d.activateAt = time.Time{}
	d.edge = hudNearestEdge(d.workArea(), hudWindowRect(d.hwnd))
	d.collapsed = true
	d.setBounds(d.collapsedBounds())
	d.evalCollapsed(true)
}

func (d *hudDock) syncInitialBounds() {
	// The embedded WebView has already adjusted the outer window. Only move the
	// surface now; applying logical bounds again would shrink it and jump DPI.
	d.arranging = true
	work := d.workArea()
	window := hudWindowRect(d.hwnd)
	width, height := window.Right-window.Left, window.Bottom-window.Top
	top := clampInt32(window.Top, work.Top+hudPixels(hudGap, d.dpi), work.Bottom-height-hudPixels(hudGap, d.dpi))
	left := work.Left + hudPixels(hudGap, d.dpi)
	win("SetWindowPos", d.hwnd, 0, uintptr(left), uintptr(top), uintptr(width), uintptr(height), 0x0016)
	d.arranging = false
	d.initialized = true
}

func (d *hudDock) setPinned(pinned bool) {
	insertAfter := uintptr(0)
	if pinned {
		insertAfter = ^uintptr(0)
	}
	win("SetWindowPos", d.hwnd, insertAfter, 0, 0, 0, 0, 0x0013) // NOSIZE | NOMOVE | NOACTIVATE
}

func hudDPI(hwnd uintptr) uint32 {
	dpi := uint32(win("GetDpiForWindow", hwnd))
	if dpi == 0 {
		dpi = 96
	}
	return dpi
}

func hudWindowRect(hwnd uintptr) rect {
	var window rect
	win("GetWindowRect", hwnd, uintptr(unsafe.Pointer(&window)))
	return window
}

func hudNearestEdge(work, window rect) int {
	left := absInt32(window.Left - work.Left)
	right := absInt32(work.Right - window.Right)
	if left < right {
		return 0
	}
	return 1
}

func absInt32(value int32) int32 {
	if value < 0 {
		return -value
	}
	return value
}

func hudPixels(value int, dpi uint32) int32 {
	return int32((value*int(dpi) + 48) / 96)
}

func (d *hudDock) workArea() rect {
	if info, ok := monitorInfoFor(d.hwnd); ok {
		return info.Work
	}
	return rect{}
}

func (d *hudDock) setBounds(r rect) {
	d.arranging = true
	defer func() { d.arranging = false }()
	win("SetWindowPos", d.hwnd, 0,
		uintptr(r.Left), uintptr(r.Top),
		uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		0x0016) // NOZORDER | NOACTIVATE | FRAMECHANGED
}

func (d *hudDock) expandedBounds() rect {
	return hudExpandedRect(d.workArea(), d.edge, d.dpi, hudWindowRect(d.hwnd))
}

func hudExpandedRect(work rect, edge int, dpi uint32, window rect) rect {
	width := hudPixels(hudExpandedWidth, dpi)
	height := hudPixels(hudExpandedHeight, dpi)
	gap := hudPixels(hudGap, dpi)
	left := window.Left
	top := window.Top
	maxLeft := work.Right - width - gap
	minLeft := work.Left + gap
	maxTop := work.Bottom - height - gap
	minTop := work.Top + gap
	if maxLeft < minLeft {
		maxLeft = minLeft
	}
	if maxTop < minTop {
		maxTop = minTop
	}
	left = clampInt32(left, minLeft, maxLeft)
	top = clampInt32(top, minTop, maxTop)
	if edge == 1 {
		left = maxLeft
	}
	if edge == 0 {
		left = minLeft
	}
	return rect{Left: left, Top: top, Right: left + width, Bottom: top + height}
}

func (d *hudDock) collapsedBounds() rect {
	return hudCollapsedRect(d.workArea(), d.edge, d.dpi, hudWindowRect(d.hwnd).Top)
}

func hudCollapsedRect(work rect, edge int, dpi uint32, top int32) rect {
	height := hudPixels(hudExpandedHeight, dpi)
	width := hudPixels(hudCollapsedWidth, dpi)
	peek := hudPixels(hudCollapsedEdge, dpi)
	gap := hudPixels(hudGap, dpi)
	top = clampInt32(top, work.Top+gap, work.Bottom-height-gap)
	if edge == 0 {
		return rect{Left: work.Left - width + peek, Top: top, Right: work.Left + peek, Bottom: top + height}
	}
	return rect{Left: work.Right - peek, Top: top, Right: work.Right + width - peek, Bottom: top + height}
}

func clampInt32(value, minValue, maxValue int32) int32 {
	if maxValue < minValue {
		return minValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (d *hudDock) snapEdge() {
	d.edge = hudNearestEdge(d.workArea(), hudWindowRect(d.hwnd))
}

func (d *hudDock) collapse() {
	if d.collapsed {
		return
	}
	d.collapsed = true
	d.setBounds(d.collapsedBounds())
	d.evalCollapsed(true)
}

func (d *hudDock) evalCollapsed(collapsed bool) {
	if d.eval == nil {
		return
	}
	value := "false"
	if collapsed {
		value = "true"
	}
	d.eval("window.xtHudCollapsed?.(" + value + ")")
}

func (d *hudDock) pointerInside() bool {
	if d.collapsed {
		return d.pointerOnPeek()
	}
	var r rect
	win("GetWindowRect", d.hwnd, uintptr(unsafe.Pointer(&r)))
	var p point
	win("GetCursorPos", uintptr(unsafe.Pointer(&p)))
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

func (d *hudDock) pointerOnPeek() bool {
	var r rect
	win("GetWindowRect", d.hwnd, uintptr(unsafe.Pointer(&r)))
	var p point
	win("GetCursorPos", uintptr(unsafe.Pointer(&p)))
	peek := hudPixels(hudCollapsedEdge+2, d.dpi)
	if d.edge == 0 {
		return p.X >= r.Right-peek && p.X <= r.Right && p.Y >= r.Top && p.Y < r.Bottom
	}
	return p.X >= r.Left && p.X <= r.Left+peek && p.Y >= r.Top && p.Y < r.Bottom
}

func (d *hudDock) watchPointer() {
	win("SetTimer", d.hwnd, 2, hudPollInterval, 0)
}

func (d *hudDock) tick() {
	if !d.activateAt.IsZero() {
		if time.Now().After(d.activateAt) {
			d.activate()
		}
		return
	}
	if d.dragging {
		return
	}
	inside := d.pointerInside()
	if inside {
		d.collapseAt = time.Time{}
		if !d.hover {
			d.hover = true
			if d.collapsed {
				d.collapsed = false
				d.setBounds(d.expandedBounds())
				d.evalCollapsed(false)
			}
		}
		return
	}
	d.hover = false
	if !d.collapsed {
		if d.collapseAt.IsZero() {
			d.collapseAt = time.Now().Add(550 * time.Millisecond)
		} else if time.Now().After(d.collapseAt) {
			d.collapse()
			d.collapseAt = time.Time{}
		}
	}
}

func (d *hudDock) handleMessage(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0005:
		d.dpi = hudDPI(h)
		if d.dragging {
			return win("CallWindowProcW", d.old, h, uintptr(msg), wp, lp)
		}
		if d.arranging {
			return 0
		}
		if lp != 0 {
			modify := (*windowPos)(lpToPointer(lp))
			modify.Flags |= 0x0010 // SWP_NOSIZE
		}
		result := win("CallWindowProcW", d.old, h, uintptr(msg), wp, lp)
		if !d.initialized {
			return result
		}
		switch {
		case d.collapsed:
			d.setBounds(d.collapsedBounds())
		case d.hover:
			d.setBounds(d.expandedBounds())
		}
		return result
	case 0x0113:
		if wp == 2 {
			d.tick()
			return 0
		}
	case 0x0084:
		if d.collapsed && d.pointerOnPeek() {
			return 1 // HTCLIENT: the page receives pointer entry and click.
		}
	case 0x0211:
		d.dragging = true
	case 0x0212:
		d.dragging = false
		if d.collapsed {
			d.collapsed = false
			d.evalCollapsed(false)
		}
		d.snapEdge()
		if d.hover {
			d.setBounds(d.expandedBounds())
		} else {
			d.collapsed = true
			d.setBounds(d.collapsedBounds())
			d.evalCollapsed(true)
		}
	case 0x0046:
		if d.dragging && lp != 0 {
			pos := (*windowPos)(lpToPointer(lp))
			work := d.workArea()
			width := hudPixels(hudExpandedWidth, d.dpi)
			height := hudPixels(hudExpandedHeight, d.dpi)
			left := clampInt32(pos.X, work.Left-hudPixels(24, d.dpi), work.Right-width+hudPixels(24, d.dpi))
			top := clampInt32(pos.Y, work.Top-hudPixels(24, d.dpi), work.Bottom-height+hudPixels(24, d.dpi))
			pos.X, pos.Y = left, top
		}
	case 0x0010:
		win("KillTimer", h, 2)
	}
	return win("CallWindowProcW", d.old, h, uintptr(msg), wp, lp)
}

func (d *hudDock) close() {
	win("KillTimer", d.hwnd, 2)
	win("SetWindowLongPtrW", d.hwnd, uintptr(uint32(0xfffffffc)), d.old)
}
