package secrets

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func protect(plain []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(plain)), Data: blobPtr(plain)}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return cloneBlob(out), nil
}

func unprotect(blob []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(blob)), Data: blobPtr(blob)}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return cloneBlob(out), nil
}

func blobPtr(b []byte) *byte {
	if len(b) == 0 {
		return nil
	}
	return &b[0]
}

func blobBytes(b windows.DataBlob) []byte {
	if b.Size == 0 {
		return nil
	}
	return unsafe.Slice(b.Data, b.Size)
}

func cloneBlob(b windows.DataBlob) []byte {
	raw := blobBytes(b)
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}
