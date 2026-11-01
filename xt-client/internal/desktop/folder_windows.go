package desktop

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

type openFileName struct {
	Size          uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustom     uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension *uint16
	DefExt        *uint16
	CustomData    uintptr
	Hook          uintptr
	TemplateName  *uint16
	ReservedPtr   uintptr
	ReservedInt   uint32
	FlagsEx       uint32
}

const folderOptionsPickFolders = 0x00000020

// PickFolder opens the native folder picker. The returned path is always absolute.
func PickFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	shell := syscall.NewLazyDLL("shell32.dll")
	ole32 := syscall.NewLazyDLL("ole32.dll")
	coInit, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0x2)
	if coInit != 0 && coInit != 1 {
		return "", fmt.Errorf("初始化文件夹选择器失败")
	}
	defer ole32.NewProc("CoUninitialize").Call()

	buf := make([]uint16, 32768)
	titleText, _ := syscall.UTF16PtrFromString(title)
	fileName := openFileName{
		Size:        uint32(unsafe.Sizeof(openFileName{})),
		Filter:      syscall.StringToUTF16Ptr("*.*\x00*.*\x00\x00"),
		File:        &buf[0],
		MaxFile:     uint32(len(buf)),
		Title:       titleText,
		Flags:       folderOptionsPickFolders,
		FilterIndex: 1,
	}
	ret, _, callErr := shell.NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&fileName)))
	if ret == 0 {
		if callErr != syscall.Errno(0) {
			return "", fmt.Errorf("已取消选择文件夹")
		}
		return "", fmt.Errorf("已取消选择文件夹")
	}
	selected := syscall.UTF16ToString(buf)
	if selected == "" {
		return "", fmt.Errorf("未选择文件夹")
	}
	absolute, err := syscall.FullPath(selected)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("所选路径不是文件夹")
	}
	return absolute, nil
}
