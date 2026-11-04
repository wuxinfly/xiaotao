//go:build !windows

package app

import "fmt"

func openProjectFolder(path string) error {
	return fmt.Errorf("当前系统不支持 Windows 资源管理器")
}
