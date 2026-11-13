package office

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestPromotionFailureRecoveryAtEachStage(t *testing.T) {
	for _, stage := range []string{"staged", "committed", "complete"} {
		t.Run(stage, func(t *testing.T) {
			store := NewRegistryStore(t.TempDir())
			project := t.TempDir()
			global, roles, team := validRegistries()
			expected := ProjectRegistries{Global: global, Project: roles, Team: team}
			saveRegistries(t, store, project, expected)
			var files = map[string][]byte{}
			var snapshot map[string][]byte
			var transactionRaw []byte
			journalPath := filepath.Join(store.dataDir, "collaboration", "transactions", "promote_injected.yaml")
			store.write = func(path string, raw []byte, mode os.FileMode) error {
				if filepath.Ext(path) == ".yaml" {
					files[path] = append([]byte(nil), raw...)
				}
				var transaction promotionTransaction
				if err := decodeTestYAML(raw, &transaction); err == nil && transaction.SchemaVersion == promotionSchemaVersion && transaction.Stage == stage {
					transactionRaw = append([]byte(nil), raw...)
					snapshot = map[string][]byte{}
					for key, value := range files {
						snapshot[key] = append([]byte(nil), value...)
					}
					return errors.New("injected transaction failure")
				}
				return writeAtomicFile(path, raw, mode)
			}
			if _, err := store.Promote(project, roles.Roles[0].ID, expected, time.Now().UTC()); err == nil {
				t.Fatal("expected injected failure")
			}
			store.write = writeAtomicFile
			if snapshot == nil || transactionRaw == nil {
				t.Fatal("no crash snapshot captured")
			}
			for path, raw := range snapshot {
				if err := writeAtomicFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeAtomicFile(journalPath, transactionRaw, 0o600); err != nil {
				t.Fatal(err)
			}
			store = NewRegistryStore(store.dataDir)
			store.recoverForTest(t)
			loaded, loadErr := store.Load(project)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			expectedScope := RoleScopeGlobal
			if stage == "staged" {
				expectedScope = RoleScopeProject
			}
			projectCount, globalCount := 0, 2
			if expectedScope == RoleScopeProject {
				projectCount, globalCount = 1, 1
			}
			if len(loaded.Project.Roles) != projectCount || len(loaded.Global.Roles) != globalCount {
				t.Fatalf("identity count: %+v", loaded)
			}
			if !memberInScope(loaded.Team, roles.Roles[0].ID, expectedScope) {
				t.Fatalf("not converged: %+v", loaded)
			}
			if expectedScope == RoleScopeGlobal && loaded.Global.Roles[1].Lifecycle != RoleActive {
				t.Fatalf("global role not active: %+v", loaded)
			}
			store.recoverForTest(t)
		})
	}
}

func TestPromotionRollbackKeepsSingleVisibleIdentity(t *testing.T) {
	store := NewRegistryStore(t.TempDir())
	project := t.TempDir()
	global, roles, team := validRegistries()
	expected := ProjectRegistries{Global: global, Project: roles, Team: team}
	saveRegistries(t, store, project, expected)
	store.write = func(path string, raw []byte, mode os.FileMode) error {
		if path == store.paths(project).team {
			return errors.New("injected team failure")
		}
		return writeAtomicFile(path, raw, mode)
	}
	if _, err := store.Promote(project, roles.Roles[0].ID, expected, time.Now()); err == nil {
		t.Fatal("expected failure")
	}
	store.write = writeAtomicFile
	loaded, err := store.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Project.Roles) != 1 || len(loaded.Global.Roles) != 1 || !memberInScope(loaded.Team, roles.Roles[0].ID, RoleScopeProject) {
		t.Fatalf("rollback invalid: %+v", loaded)
	}
	if entries, err := os.ReadDir(filepath.Join(store.dataDir, "collaboration", "transactions")); err == nil && len(entries) != 0 {
		t.Fatal("journal not removed")
	}
}

func (s *RegistryStore) recoverForTest(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverTransactionsLocked(); err != nil {
		t.Fatal(err)
	}
}

func decodeTestYAML(raw []byte, target any) error { return yaml.Unmarshal(raw, target) }
