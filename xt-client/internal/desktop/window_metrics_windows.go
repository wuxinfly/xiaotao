package desktop

import "golang.org/x/sys/windows"

type monitorInfo struct {
	Size    uint32
	Monitor windows.Rect
	Work    windows.Rect
	Flags   uint32
}

// MINMAXINFO uses coordinates relative to the monitor, not the virtual desktop.
type minMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize point
}

func constrainMaximizedWindow(limits *minMaxInfo, info monitorInfo) {
	limits.MaxPosition = point{info.Work.Left - info.Monitor.Left, info.Work.Top - info.Monitor.Top}
	limits.MaxSize = point{info.Work.Right - info.Work.Left, info.Work.Bottom - info.Work.Top}
}
