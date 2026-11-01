package runtimeenv

import (
	"path/filepath"
	"strings"
)

const (
	Name = "xiaotao"
	UAT  = "uat"
)

type Environment struct {
	Name             string
	DisplayName      string
	DataDirName      string
	DefaultPort      string
	MutexNamespace   string
	ShortcutFileName string
	ModelSourceDir   string
}

// Active is set by build.ps1 through -ldflags. An empty value keeps the
// production defaults for existing developer builds.
var Active = ""

func Current() Environment {
	if strings.EqualFold(strings.TrimSpace(Active), UAT) {
		return Environment{
			Name:             UAT,
			DisplayName:      "小涛 UAT",
			DataDirName:      "XiaoTao-uat",
			DefaultPort:      "5456",
			MutexNamespace:   "XiaoTao.UAT",
			ShortcutFileName: "小涛 UAT.lnk",
			ModelSourceDir:   "XiaoTao",
		}
	}
	return Environment{
		Name:             Name,
		DisplayName:      "小涛",
		DataDirName:      "XiaoTao",
		DefaultPort:      "5455",
		MutexNamespace:   "XiaoTao",
		ShortcutFileName: "小涛.lnk",
	}
}

func DefaultDataDir(localAppData string) string {
	return filepath.Join(localAppData, Current().DataDirName)
}

func DefaultModelSourceDir(localAppData string) string {
	source := Current().ModelSourceDir
	if source == "" {
		return ""
	}
	return filepath.Join(localAppData, source)
}

func ServiceMutex(dataDir string) string {
	return "Local\\" + Current().MutexNamespace + ".Service." + dataDir
}

func WindowMutex(dataDir string) string {
	return "Local\\" + Current().MutexNamespace + ".Window." + dataDir
}
