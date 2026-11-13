package office

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTeamOnboardingPersistsSkipPerProject(t *testing.T) {
	service := NewTeamService(NewRegistryStore(t.TempDir()))
	project, other := t.TempDir(), t.TempDir()
	empty, err := service.Snapshot(project)
	if err != nil || empty.Onboarding != OnboardingPending {
		t.Fatalf("new project must show onboarding: %+v, %v", empty, err)
	}
	if _, err = service.SkipOnboarding(other, ProjectRegistries{}); err != nil {
		t.Fatal(err)
	}
	if skipped, err := service.Snapshot(other); err != nil || skipped.Onboarding != OnboardingSkipped {
		t.Fatalf("skip was not persisted: %+v, %v", skipped, err)
	}
	unchanged, err := service.Snapshot(project)
	if err != nil || unchanged.Onboarding != OnboardingPending {
		t.Fatalf("skip leaked to another project: %+v, %v", unchanged, err)
	}
	created, err := service.Bootstrap(project, TeamTemplates()[0].Roles, revisions(empty))
	if err != nil || created.Onboarding != OnboardingCompleted {
		t.Fatalf("a real team must skip onboarding: %+v, %v", created, err)
	}
	legacy, err := os.ReadFile(filepath.Join(project, ".xiaotao", "team.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	legacy = bytes.Replace(legacy, []byte("onboarding: pending\n"), nil, 1)
	if err = os.WriteFile(filepath.Join(project, ".xiaotao", "team.yaml"), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Snapshot(project)
	if err != nil || loaded.Onboarding != OnboardingCompleted {
		t.Fatalf("legacy team with members regressed to onboarding: %+v, %v", loaded, err)
	}
}

func TestSkipOnboardingRejectsConflictAndCorruptState(t *testing.T) {
	service := NewTeamService(NewRegistryStore(t.TempDir()))
	project := t.TempDir()
	if _, err := service.SkipOnboarding(project, ProjectRegistries{Team: TeamRegistry{Revision: 1}}); err != ErrRoleConflict {
		t.Fatalf("stale revision: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".xiaotao"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".xiaotao", "team.yaml"), []byte("schema_version: 1\nonboarding: maybe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SkipOnboarding(project, ProjectRegistries{}); err == nil {
		t.Fatal("corrupt onboarding accepted")
	}
}
