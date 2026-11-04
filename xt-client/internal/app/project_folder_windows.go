package app

import "golang.org/x/sys/windows"

func openProjectFolder(path string) error {
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("explore"), target, nil, nil, 1)
}
