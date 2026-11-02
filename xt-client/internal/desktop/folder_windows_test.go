package desktop

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestCallCOMDispatchesVtableMethod(t *testing.T) {
	var received uintptr
	callback := syscall.NewCallback(func(this, value uintptr) uintptr {
		received = value
		return 42
	})
	vtable := [2]uintptr{callback, callback}
	object := unsafe.Pointer(&vtable[0])
	if got := callCOM(unsafe.Pointer(&object), unsafe.Sizeof(uintptr(0)), 123); got != 42 || received != 123 {
		t.Fatalf("callCOM returned %d and passed %d; want 42 and 123", got, received)
	}
	if got := callCOM(unsafe.Pointer(&object), 0, 456); got != 42 || received != 456 {
		t.Fatalf("first vtable slot returned %d and passed %d", got, received)
	}
}
