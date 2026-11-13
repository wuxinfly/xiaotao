package office

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func saveRegistries(t *testing.T, store *RegistryStore, project string, value ProjectRegistries) {
	t.Helper()
	paths := store.paths(project)
	if err := store.writeRegistry(paths.global, value.Global); err != nil {
		t.Fatal(err)
	}
	if err := store.writeRegistry(paths.project, value.Project); err != nil {
		t.Fatal(err)
	}
	if err := store.writeRegistry(paths.team, value.Team); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRoundTripIsolationAndStrictFields(t *testing.T) {
	production := NewRegistryStore(filepath.Join(t.TempDir(), "XiaoTao"))
	uat := NewRegistryStore(filepath.Join(t.TempDir(), "XiaoTao-uat"))
	first, second := t.TempDir(), t.TempDir()
	global, project, team := validRegistries()
	value := ProjectRegistries{Global: global, Project: project, Team: team}
	saveRegistries(t, production, first, value)
	saveRegistries(t, uat, first, value)
	memory := []byte("keep this project memory")
	memoryPath := filepath.Join(first, ".xiaotao", "STATE.md")
	if err := os.WriteFile(memoryPath, memory, 0o600); err != nil {
		t.Fatal(err)
	}
	value.Project.Roles[0].Name = "仅第一个项目"
	value.Project.Revision++
	value.Global.Roles[0].Name = "仅正式版"
	value.Global.Revision++
	if err := production.writeRegistry(production.paths(first).project, value.Project); err != nil {
		t.Fatal(err)
	}
	if err := production.writeRegistry(production.paths(first).global, value.Global); err != nil {
		t.Fatal(err)
	}
	loaded, err := production.Load(first)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.Roles[0].Name != "仅第一个项目" {
		t.Fatal("round trip lost role")
	}
	secondLoaded, err := production.Load(second)
	if err != nil || len(secondLoaded.Project.Roles) != 0 {
		t.Fatalf("project role leaked: %+v %v", secondLoaded, err)
	}
	uatLoaded, err := uat.Load(first)
	if err != nil || uatLoaded.Global.Roles[0].Name != "产品" {
		t.Fatalf("environment role changed: %v", err)
	}
	actualMemory, err := os.ReadFile(memoryPath)
	if err != nil || string(actualMemory) != string(memory) {
		t.Fatal("memory file overwritten")
	}
	unknown := strings.Replace(string(mustRead(t, production.paths(first).project)), "revision:", "unexpected: true\nrevision:", 1)
	if err = os.WriteFile(production.paths(first).project, []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = production.Load(first); !errors.Is(err, ErrRoleCorrupt) {
		t.Fatalf("unknown field accepted: %v", err)
	}
}

func TestRegistryFailurePreservesOriginal(t *testing.T) {
	store := NewRegistryStore(t.TempDir())
	project := t.TempDir()
	global, roles, team := validRegistries()
	saveRegistries(t, store, project, ProjectRegistries{Global: global, Project: roles, Team: team})
	path := store.paths(project).project
	original := mustRead(t, path)
	store.write = func(string, []byte, os.FileMode) error { return errors.New("injected write failure") }
	roles.Revision++
	if err := store.writeRegistry(path, roles); err == nil {
		t.Fatal("expected write failure")
	}
	if string(mustRead(t, path)) != string(original) {
		t.Fatal("original registry changed")
	}
}

func TestRegistryUpdateRejectsStaleRevision(t *testing.T) {
	store := NewRegistryStore(t.TempDir())
	project := t.TempDir()
	global, roles, team := validRegistries()
	expected := ProjectRegistries{Global: global, Project: roles, Team: team}
	saveRegistries(t, store, project, expected)
	_, err := store.Update(project, expected, func(value *ProjectRegistries) error {
		value.Project.Revision++
		value.Project.Roles[0].Name = "已保存"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(project, expected, func(value *ProjectRegistries) error {
		value.Project.Revision++
		value.Project.Roles[0].Name = "不应保存"
		return nil
	})
	if !errors.Is(err, ErrRoleConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	loaded, loadErr := store.Load(project)
	if loadErr != nil || loaded.Project.Roles[0].Name != "已保存" {
		t.Fatalf("stale write changed data: %v", loadErr)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
