package office

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentedRegistryExamples(t *testing.T) {
	directory := filepath.Join("..", "..", "..", "docs", "office", "roles")
	var global, project RoleRegistry
	var team TeamRegistry
	for path, target := range map[string]any{"global-registry.example.yaml": &global, "project-registry.example.yaml": &project, "team.example.yaml": &team} {
		raw, err := os.ReadFile(filepath.Join(directory, path))
		if err != nil {
			t.Fatal(err)
		}
		if err = readYAMLBytes(raw, target); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if err := ValidateRoleRegistry(global, RoleScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoleRegistry(project, RoleScopeProject); err != nil {
		t.Fatal(err)
	}
	team.Members[0].RoleID = project.Roles[0].ID
	if err := ValidateTeam(team, global, project); err != nil {
		t.Fatal(err)
	}
}
