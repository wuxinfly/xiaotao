package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreBundleAndLifecycle(t *testing.T) {
	data := t.TempDir()
	initial, err := prepareCore(data, true)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile("../../../xt-skills/xiaotao/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initial, string(canonical)) {
		t.Fatal("first request must contain the complete canonical Skill")
	}
	entries, err := os.ReadDir(filepath.Join(data, "core"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("Core installation: %v, %v", entries, err)
	}
	root := filepath.Join(data, "core", entries[0].Name())
	for _, name := range []string{"SKILL.md", "references/progress-saving.md", "references/storage.md", "scripts/save_progress.py", "scripts/memory_catalog.py"} {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("../../../xt-skills/xiaotao", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("stale bundle: %s; run go generate ./internal/agent", name)
		}
	}
	// Repair a modified installed script before instructing the model to use it.
	target := filepath.Join(root, "scripts", "save_progress.py")
	if err := os.WriteFile(target, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := prepareCore(data, false)
	if err != nil {
		t.Fatal(err)
	}
	repaired, _ := os.ReadFile(target)
	if string(repaired) == "modified" {
		t.Fatal("modified script was trusted")
	}
	if strings.Contains(next, string(canonical)) || !strings.Contains(next, "持续守卫") || !strings.Contains(next, root) {
		t.Fatal("later turns must use guard and on-demand Core")
	}
	other := t.TempDir()
	otherPrompt, err := prepareCore(other, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(otherPrompt, root) || !strings.Contains(otherPrompt, other) {
		t.Fatal("environments share Core paths")
	}
}
