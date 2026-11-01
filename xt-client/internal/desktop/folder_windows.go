package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32 = windows.NewLazySystemDLL("ole32.dll")
	modShell = windows.NewLazySystemDLL("shell32.dll")

	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
)

var (
	clsidFileOpenDialog = windows.GUID{
		Data1: 0xDC1C5A9C,
		Data2: 0xE88A,
		Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7},
	}
	iidIFileOpenDialog = windows.GUID{
		Data1: 0xD57C7288,
		Data2: 0xD4AD,
		Data3: 0x4768,
		Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60},
	}
)

const (
	coinitApartmentThreaded = 0x2
	rpcEChangedMode         = 0x80010106
	fosPickFolders          = 0x00000020
	fosForceFileSystem      = 0x00000040
	fosPathMustExist        = 0x00000800
	sigdnFileSystemPath     = 0x80058000
	hrCanceled              = 0x800704C7
)

// These are only used through unsafe.Offsetof, so their field order must
// match the Windows C vtable exactly. Method pointers are never called.
type iUnknownVTbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
}

type iFileOpenDialogVTbl struct {
	iUnknownVTbl
	Show                uintptr
	SetFileTypes        uintptr
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	AddPlace            uintptr
	SetDefaultExtension uintptr
	Close               uintptr
	SetClientGuid       uintptr
	ClearClientData     uintptr
	SetFilter           uintptr
	GetResults          uintptr
	GetSelectedItems    uintptr
}

type iShellItemVTbl struct {
	iUnknownVTbl
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

func callCOM(obj, method uintptr, args ...uintptr) uintptr {
	if obj == 0 || method == 0 {
		return 0x80004003
	}
	vtblOwner := *(*uintptr)(unsafe.Pointer(&obj))
	vtbl := *(*uintptr)(unsafe.Pointer(&vtblOwner))
	if vtbl == 0 {
		return 0x80004003
	}
	slot := vtbl + method
	fn := *(*uintptr)(unsafe.Pointer(&slot))
	if fn == 0 {
		return 0x80004003
	}
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func hresultFailed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

// PickFolder opens the native folder-only picker. The returned path is always absolute.
func PickFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if hresultFailed(hr) && uint32(hr) != rpcEChangedMode {
		return "", fmt.Errorf("初始化文件夹选择器失败: 0x%08X", uint32(hr))
	}
	if uint32(hr) != rpcEChangedMode {
		defer procCoUninitialize.Call()
	}

	var dialog uintptr
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		0x1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if hresultFailed(hr) || dialog == 0 {
		return "", fmt.Errorf("创建文件夹选择器失败: 0x%08X", uint32(hr))
	}
	defer callCOM(dialog, unsafe.Offsetof(iUnknownVTbl{}.Release))

	var options uint32
	hr = callCOM(dialog, unsafe.Offsetof(iFileOpenDialogVTbl{}.GetOptions), uintptr(unsafe.Pointer(&options)))
	if hresultFailed(hr) {
		return "", fmt.Errorf("读取文件夹选择器选项失败: 0x%08X", uint32(hr))
	}
	options |= fosPickFolders | fosForceFileSystem | fosPathMustExist
	hr = callCOM(dialog, unsafe.Offsetof(iFileOpenDialogVTbl{}.SetOptions), uintptr(options))
	if hresultFailed(hr) {
		return "", fmt.Errorf("设置文件夹选择器选项失败: 0x%08X", uint32(hr))
	}
	if title != "" {
		titlePtr, _ := syscall.UTF16PtrFromString(title)
		hr = callCOM(dialog, unsafe.Offsetof(iFileOpenDialogVTbl{}.SetTitle), uintptr(unsafe.Pointer(titlePtr)))
		if hresultFailed(hr) {
			return "", fmt.Errorf("设置文件夹选择器标题失败: 0x%08X", uint32(hr))
		}
	}
	hr = callCOM(dialog, unsafe.Offsetof(iFileOpenDialogVTbl{}.Show), 0)
	if hresultFailed(hr) {
		if uint32(hr) == hrCanceled {
			return "", fmt.Errorf("已取消选择文件夹")
		}
		return "", fmt.Errorf("打开文件夹选择器失败: 0x%08X", uint32(hr))
	}

	var item uintptr
	hr = callCOM(dialog, unsafe.Offsetof(iFileOpenDialogVTbl{}.GetResult), uintptr(unsafe.Pointer(&item)))
	if hresultFailed(hr) || item == 0 {
		return "", fmt.Errorf("读取所选文件夹失败: 0x%08X", uint32(hr))
	}
	defer callCOM(item, unsafe.Offsetof(iUnknownVTbl{}.Release))

	var rawPath *uint16
	hr = callCOM(item, unsafe.Offsetof(iShellItemVTbl{}.GetDisplayName), sigdnFileSystemPath, uintptr(unsafe.Pointer(&rawPath)))
	if hresultFailed(hr) || rawPath == nil {
		return "", fmt.Errorf("读取文件夹路径失败: 0x%08X", uint32(hr))
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(rawPath))

	selected := windows.UTF16PtrToString(rawPath)
	if selected == "" {
		return "", fmt.Errorf("未选择文件夹")
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("所选文件夹无法访问: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("所选路径不是文件夹")
	}
	return filepath.Clean(absolute), nil
}
