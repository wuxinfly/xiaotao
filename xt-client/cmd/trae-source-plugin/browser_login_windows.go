package main

import "golang.org/x/sys/windows"

func openLoginBrowser(url string) error {
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(url), nil, nil, 1)
}
