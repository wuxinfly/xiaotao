package runtimeenv

import (
	"path/filepath"
	"testing"
)

func TestProductionDefaults(t *testing.T) {
	Active = ""
	t.Cleanup(func() { Active = "" })
	got := Current()
	if got.Name != Name || got.DataDirName != "XiaoTao" || got.DefaultPort != "5455" {
		t.Fatalf("unexpected production defaults: %#v", got)
	}
	if filepath.Base(DefaultDataDir(`C:\Users\test\AppData\Local`)) != "XiaoTao" {
		t.Fatal("production data directory changed")
	}
	if DefaultModelSourceDir(`C:\Users\test\AppData\Local`) != "" {
		t.Fatal("production must not import model accounts")
	}
}

func TestUATIsolation(t *testing.T) {
	Active = "uat"
	t.Cleanup(func() { Active = "" })
	got := Current()
	if got.Name != UAT || got.DisplayName != "小涛 UAT" || got.DataDirName != "XiaoTao-uat" || got.DefaultPort != "5456" {
		t.Fatalf("unexpected UAT defaults: %#v", got)
	}
	if filepath.Base(DefaultDataDir(`C:\Users\test\AppData\Local`)) != "XiaoTao-uat" {
		t.Fatal("UAT data directory changed")
	}
	modelSource := DefaultModelSourceDir(`C:\Users\test\AppData\Local`)
	if modelSource != filepath.Join(`C:\Users\test\AppData\Local`, "XiaoTao") {
		t.Fatalf("unexpected UAT model source directory: %q", modelSource)
	}
	if !filepath.IsAbs(modelSource) {
		t.Fatal("UAT model source directory is not absolute")
	}
	if ServiceMutex("a") == WindowMutex("a") || ServiceMutex("a") == LaunchMutex("a") || WindowMutex("a") == LaunchMutex("a") || got.MutexNamespace != "XiaoTao.UAT" {
		t.Fatal("UAT mutexes are not isolated")
	}
}
