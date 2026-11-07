package desktop

import "testing"

func TestHUDCollapsedBoundsKeepOnlyPeekVisible(t *testing.T) {
	dock := hudDock{edge: 0, dpi: 96}
	work := rect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	got := hudCollapsedRect(work, dock.edge, dock.dpi, 300)
	if got.Left >= work.Left || got.Right-work.Left != hudCollapsedEdge {
		t.Fatalf("left dock should expose only %d px: %+v", hudCollapsedEdge, got)
	}
	if got.Top != 300 || got.Bottom-got.Top != hudExpandedHeight {
		t.Fatalf("left dock vertical bounds changed: %+v", got)
	}
}

func TestHUDCollapsedBoundsOnRightMonitor(t *testing.T) {
	work := rect{Left: -1280, Top: -100, Right: 0, Bottom: 924}
	got := hudCollapsedRect(work, 1, 96, 250)
	if got.Left != work.Right-hudCollapsedEdge || got.Right <= work.Right {
		t.Fatalf("right dock should expose only %d px: %+v", hudCollapsedEdge, got)
	}
	if got.Top != 250 {
		t.Fatalf("right dock vertical position changed: %+v", got)
	}
}

func TestHUDExpandedBoundsStayOnCurrentMonitor(t *testing.T) {
	work := rect{Left: 1920, Top: 200, Right: 3520, Bottom: 1100}
	got := hudExpandedRect(work, 0, 96, rect{Left: 2050, Top: 950, Right: 2350, Bottom: 1150})
	if got.Left < work.Left+hudGap || got.Right > work.Right {
		t.Fatalf("expanded window escaped work area horizontally: %+v", got)
	}
	if got.Top < work.Top+hudGap || got.Bottom > work.Bottom {
		t.Fatalf("expanded window escaped work area vertically: %+v", got)
	}
}
