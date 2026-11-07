package desktop

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestConstrainMaximizedWindow(t *testing.T) {
	tests := []struct {
		name           string
		monitor, work  windows.Rect
		position, size point
	}{
		{"bottom taskbar", windows.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080}, windows.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}, point{0, 0}, point{1920, 1040}},
		{"left taskbar on secondary monitor", windows.Rect{Left: -1280, Top: -100, Right: 0, Bottom: 924}, windows.Rect{Left: -1240, Top: -100, Right: 0, Bottom: 924}, point{40, 0}, point{1240, 1024}},
		{"top taskbar", windows.Rect{Left: 1920, Top: 200, Right: 3520, Bottom: 1100}, windows.Rect{Left: 1920, Top: 240, Right: 3520, Bottom: 1100}, point{0, 40}, point{1600, 860}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := minMaxInfo{MinTrackSize: point{300, 200}}
			constrainMaximizedWindow(&limits, monitorInfo{Monitor: tt.monitor, Work: tt.work})
			if limits.MaxPosition != tt.position || limits.MaxSize != tt.size || limits.MinTrackSize != (point{300, 200}) {
				t.Fatalf("unexpected limits: %+v", limits)
			}
		})
	}
}
