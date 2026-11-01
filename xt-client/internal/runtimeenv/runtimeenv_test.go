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
	if ServiceMutex("a") == WindowMutex("a") || got.MutexNamespace != "XiaoTao.UAT" {
		t.Fatal("UAT mutexes are not isolated")
	}
}
