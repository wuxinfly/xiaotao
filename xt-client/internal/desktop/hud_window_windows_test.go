package desktop

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestHUDWindowLookupIgnoresOtherWindowClasses(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	title := "xiaotao-hud-lookup-test"
	hwnd := win("CreateWindowExW", 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(title))), 0, 0, 0, 100, 100, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal("create unrelated window")
	}
	defer win("DestroyWindow", hwnd)
	if WindowExists(title) || CloseWindow(title) {
		t.Fatal("unrelated window must not be treated as a desktop window")
	}
}
