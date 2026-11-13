package office

import (
	"errors"
	"testing"
	"time"
)

func testRole(id string) Role {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	return Role{ID: id, Scope: RoleScopeProject, Lifecycle: RoleActive, Name: "产品", Responsibility: "梳理需求", Capabilities: []string{"分析"}, Appearance: Appearance{Shape: "round", PrimaryColor: "#36a9c9", SecondaryColor: "#173b55"}, CreatedAt: now, UpdatedAt: now}
}

func validRegistries() (RoleRegistry, RoleRegistry, TeamRegistry) {
	role := testRole("role_0123456789abcdef")
	globalRole := role
	globalRole.ID = "role_fedcba9876543210"
	globalRole.Scope = RoleScopeGlobal
	return RoleRegistry{SchemaVersion: 1, Revision: 1, Roles: []Role{globalRole}}, RoleRegistry{SchemaVersion: 1, Revision: 1, Roles: []Role{role}}, TeamRegistry{SchemaVersion: 1, Revision: 1, Members: []Membership{{RoleID: role.ID, RoleScope: RoleScopeProject, JoinedAt: role.CreatedAt}}}
}

func TestRoleRegistryStrictValidation(t *testing.T) {
	global, project, _ := validRegistries()
	cases := map[string]func() RoleRegistry{
		"duplicate": func() RoleRegistry {
			value := project
			value.Roles = append(value.Roles, project.Roles[0])
			return value
		},
		"scope": func() RoleRegistry {
			value := project
			value.Roles = append([]Role(nil), project.Roles...)
			value.Roles[0].Scope = RoleScopeGlobal
			return value
		},
		"appearance": func() RoleRegistry {
			value := project
			value.Roles = append([]Role(nil), project.Roles...)
			value.Roles[0].Appearance.PrimaryColor = "blue"
			return value
		},
		"version": func() RoleRegistry { value := project; value.SchemaVersion = 2; return value },
		"model": func() RoleRegistry {
			value := project
			value.Roles = append([]Role(nil), project.Roles...)
			value.Roles[0].Model = ModelPreference{ContextWindow: 1024, MaxOutputTokens: 1024}
			return value
		},
	}
	for name, registry := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRoleRegistry(registry(), RoleScopeProject); !errors.Is(err, ErrRoleCorrupt) {
				t.Fatalf("expected corruption, got %v", err)
			}
		})
	}
	if err := ValidateRoleRegistry(global, RoleScopeGlobal); err != nil || ValidateRoleRegistry(project, RoleScopeProject) != nil {
		t.Fatalf("valid registries rejected: %v", err)
	}
}

func TestTeamRequiresUniqueActiveMatchingRoles(t *testing.T) {
	global, project, team := validRegistries()
	for name, mutate := range map[string]func(*RoleRegistry, *TeamRegistry){
		"missing":   func(_ *RoleRegistry, value *TeamRegistry) { value.Members[0].RoleID = "role_aaaaaaaaaaaaaaaa" },
		"scope":     func(_ *RoleRegistry, value *TeamRegistry) { value.Members[0].RoleScope = RoleScopeGlobal },
		"staged":    func(value *RoleRegistry, _ *TeamRegistry) { value.Roles[0].Lifecycle = RoleStaged },
		"duplicate": func(_ *RoleRegistry, value *TeamRegistry) { value.Members = append(value.Members, value.Members[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			project, team := project, team
			mutate(&project, &team)
			if err := ValidateTeam(team, global, project); !errors.Is(err, ErrRoleCorrupt) {
				t.Fatalf("expected corruption, got %v", err)
			}
		})
	}
}

func TestRoleAndTeamCapacity(t *testing.T) {
	global, project, team := validRegistries()
	team.Members = make([]Membership, MaxProjectMembers+1)
	if err := ValidateTeam(team, global, project); !errors.Is(err, ErrRoleCorrupt) {
		t.Fatalf("expected member capacity error, got %v", err)
	}
	project.Roles = make([]Role, MaxRegistryRoles+1)
	if err := ValidateRoleRegistry(project, RoleScopeProject); !errors.Is(err, ErrRoleCorrupt) {
		t.Fatalf("expected role capacity error, got %v", err)
	}
}
