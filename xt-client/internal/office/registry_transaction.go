package office

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const promotionSchemaVersion = 1

type promotionTransaction struct {
	SchemaVersion   int       `yaml:"schema_version"`
	ID              string    `yaml:"id"`
	ProjectPath     string    `yaml:"project_path"`
	Role            Role      `yaml:"role"`
	ExpectedGlobal  int64     `yaml:"expected_global_revision"`
	ExpectedProject int64     `yaml:"expected_project_revision"`
	ExpectedTeam    int64     `yaml:"expected_team_revision"`
	GlobalRevision  int64     `yaml:"global_revision"`
	ProjectRevision int64     `yaml:"project_revision"`
	TeamRevision    int64     `yaml:"team_revision"`
	Stage           string    `yaml:"stage"`
	CreatedAt       time.Time `yaml:"created_at"`
}

func (s *RegistryStore) Promote(projectPath, roleID string, expected ProjectRegistries, now time.Time) (ProjectRegistries, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.recoverTransactionsLocked(); err != nil {
		return ProjectRegistries{}, err
	}
	current, err := s.loadLocked(projectPath)
	if err != nil {
		return ProjectRegistries{}, err
	}
	if current.Global.Revision != expected.Global.Revision || current.Project.Revision != expected.Project.Revision || current.Team.Revision != expected.Team.Revision {
		return ProjectRegistries{}, ErrRoleConflict
	}
	index, role := findRole(current.Project.Roles, roleID)
	if index < 0 || role.Lifecycle != RoleActive || !memberInScope(current.Team, roleID, RoleScopeProject) {
		return ProjectRegistries{}, ErrRoleNotFound
	}
	transaction := promotionTransaction{SchemaVersion: promotionSchemaVersion, ID: "promote_" + randomID(8), ProjectPath: projectPath, Role: role, ExpectedGlobal: current.Global.Revision, ExpectedProject: current.Project.Revision, ExpectedTeam: current.Team.Revision, Stage: "prepared", CreatedAt: now.UTC()}
	journalPath := filepath.Join(s.dataDir, "collaboration", "transactions", transaction.ID+".yaml")
	if err = s.writeRegistry(journalPath, transaction); err != nil {
		return ProjectRegistries{}, err
	}
	if err = s.registerProject(projectPath); err != nil {
		_ = os.Remove(journalPath)
		return ProjectRegistries{}, err
	}
	committed, err := s.applyPromotion(projectPath, &transaction, journalPath)
	if err != nil {
		if recoveryErr := s.recoverPromotion(transaction, journalPath); recoveryErr != nil {
			return ProjectRegistries{}, fmt.Errorf("提升失败且恢复未完成: %v; %w", err, recoveryErr)
		}
		return ProjectRegistries{}, err
	}
	return committed, nil
}

func (s *RegistryStore) applyPromotion(projectPath string, transaction *promotionTransaction, journalPath string) (ProjectRegistries, error) {
	current, err := s.loadForRecoveryLocked(projectPath, transaction.Role.ID)
	if err != nil {
		return ProjectRegistries{}, err
	}
	paths := s.paths(projectPath)
	if transaction.Stage == "prepared" {
		globalRole := transaction.Role
		globalRole.Scope, globalRole.Lifecycle, globalRole.UpdatedAt = RoleScopeGlobal, RoleStaged, transaction.CreatedAt
		current.Global.Roles = append(current.Global.Roles, globalRole)
		current.Global.Revision++
		transaction.GlobalRevision = current.Global.Revision
		if err = s.writeRegistry(paths.global, current.Global); err != nil {
			return ProjectRegistries{}, err
		}
		transaction.Stage = "staged"
		if err = s.writeRegistry(journalPath, transaction); err != nil {
			return ProjectRegistries{}, err
		}
	}
	if transaction.Stage == "staged" {
		for index := range current.Team.Members {
			if current.Team.Members[index].RoleID == transaction.Role.ID {
				current.Team.Members[index].RoleScope = RoleScopeGlobal
			}
		}
		current.Team.Revision++
		transaction.TeamRevision = current.Team.Revision
		if err = s.writeRegistry(paths.team, current.Team); err != nil {
			return ProjectRegistries{}, err
		}
		transaction.Stage = "committed"
		if err = s.writeRegistry(journalPath, transaction); err != nil {
			return ProjectRegistries{}, err
		}
	}
	if transaction.Stage == "committed" {
		for index := range current.Global.Roles {
			if current.Global.Roles[index].ID == transaction.Role.ID {
				current.Global.Roles[index].Lifecycle = RoleActive
				current.Global.Roles[index].UpdatedAt = transaction.CreatedAt
			}
		}
		current.Global.Revision++
		current.Project.Roles = removeRole(current.Project.Roles, transaction.Role.ID)
		current.Project.Revision++
		if err = s.writeRegistry(paths.global, current.Global); err != nil {
			return ProjectRegistries{}, err
		}
		if err = s.writeRegistry(paths.project, current.Project); err != nil {
			return ProjectRegistries{}, err
		}
		transaction.Stage = "complete"
		if err = s.writeRegistry(journalPath, transaction); err != nil {
			return ProjectRegistries{}, err
		}
		if err = os.Remove(journalPath); err != nil {
			return ProjectRegistries{}, err
		}
	}
	return s.loadLocked(projectPath)
}

func (s *RegistryStore) rollbackPromotion(projectPath string, transaction promotionTransaction) error {
	current, err := s.loadLocked(projectPath)
	if err != nil {
		return err
	}
	if memberInScope(current.Team, transaction.Role.ID, RoleScopeGlobal) {
		return errors.New("成员归属已提交，应执行前向恢复")
	}
	paths := s.paths(projectPath)
	if roleExists(current.Global.Roles, transaction.Role.ID) {
		current.Global.Roles = removeRole(current.Global.Roles, transaction.Role.ID)
		current.Global.Revision++
		if err = s.writeRegistry(paths.global, current.Global); err != nil {
			return err
		}
	}
	if !roleExists(current.Project.Roles, transaction.Role.ID) {
		current.Project.Roles = append(current.Project.Roles, transaction.Role)
		current.Project.Revision++
		if err = s.writeRegistry(paths.project, current.Project); err != nil {
			return err
		}
	}
	return nil
}

func (s *RegistryStore) recoverTransactionsLocked() error {
	directory := filepath.Join(s.dataDir, "collaboration", "transactions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		journalPath := filepath.Join(directory, entry.Name())
		var transaction promotionTransaction
		if err = readYAML(journalPath, &transaction); err != nil {
			return err
		}
		if transaction.SchemaVersion != promotionSchemaVersion || !roleIDPattern.MatchString(transaction.Role.ID) || transaction.ProjectPath == "" {
			return fmt.Errorf("%w: 无效提升事务 %s", ErrRoleCorrupt, journalPath)
		}
		current, loadErr := s.loadForRecoveryLocked(transaction.ProjectPath, transaction.Role.ID)
		if loadErr != nil {
			return loadErr
		}
		if memberInScope(current.Team, transaction.Role.ID, RoleScopeGlobal) {
			transaction.Stage = "committed"
			if _, err = s.finishPromotionRecovery(transaction.ProjectPath, &transaction, journalPath); err != nil {
				return err
			}
		} else {
			if err = s.rollbackPromotion(transaction.ProjectPath, transaction); err != nil {
				return err
			}
			if err = os.Remove(journalPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *RegistryStore) finishPromotionRecovery(projectPath string, transaction *promotionTransaction, journalPath string) (ProjectRegistries, error) {
	current, err := s.loadForRecoveryLocked(projectPath, transaction.Role.ID)
	if err != nil {
		return ProjectRegistries{}, err
	}
	paths := s.paths(projectPath)
	role := transaction.Role
	role.Scope, role.Lifecycle, role.UpdatedAt = RoleScopeGlobal, RoleActive, transaction.CreatedAt
	if roleIndex, _ := findRole(current.Global.Roles, role.ID); roleIndex >= 0 {
		current.Global.Roles[roleIndex] = role
	} else {
		current.Global.Roles = append(current.Global.Roles, role)
	}
	current.Global.Revision++
	if err = s.writeRegistry(paths.global, current.Global); err != nil {
		return ProjectRegistries{}, err
	}
	if roleExists(current.Project.Roles, role.ID) {
		current.Project.Roles = removeRole(current.Project.Roles, role.ID)
		current.Project.Revision++
		if err = s.writeRegistry(paths.project, current.Project); err != nil {
			return ProjectRegistries{}, err
		}
	}
	if err = os.Remove(journalPath); err != nil {
		return ProjectRegistries{}, err
	}
	return s.loadLocked(projectPath)
}

func (s *RegistryStore) recoverPromotion(transaction promotionTransaction, journalPath string) error {
	current, err := s.loadForRecoveryLocked(transaction.ProjectPath, transaction.Role.ID)
	if err != nil {
		return err
	}
	if memberInScope(current.Team, transaction.Role.ID, RoleScopeGlobal) {
		_, err = s.finishPromotionRecovery(transaction.ProjectPath, &transaction, journalPath)
		return err
	}
	if err = s.rollbackPromotion(transaction.ProjectPath, transaction); err != nil {
		return err
	}
	return os.Remove(journalPath)
}

func (s *RegistryStore) loadForRecoveryLocked(projectPath, roleID string) (ProjectRegistries, error) {
	paths := s.paths(projectPath)
	global, globalErr := readRoles(paths.global, RoleScopeGlobal)
	project, projectErr := readRoles(paths.project, RoleScopeProject)
	team, teamErr := readTeam(paths.team)
	if globalErr != nil || projectErr != nil || teamErr != nil {
		if globalErr != nil {
			return ProjectRegistries{}, globalErr
		}
		if projectErr != nil {
			return ProjectRegistries{}, projectErr
		}
		return ProjectRegistries{}, teamErr
	}
	if err := ValidateTeam(team, global, project); err == nil {
		return ProjectRegistries{Global: global, Project: project, Team: team}, nil
	}
	globalRole, projectRole := roleExists(global.Roles, roleID), roleExists(project.Roles, roleID)
	memberScope := ""
	for _, member := range team.Members {
		if member.RoleID == roleID {
			memberScope = member.RoleScope
		}
	}
	if memberScope == RoleScopeProject && (!globalRole || !projectRole) || memberScope == RoleScopeGlobal && (!globalRole && !projectRole) {
		return ProjectRegistries{}, fmt.Errorf("%w: 提升事务没有匹配的暂存身份", ErrRoleCorrupt)
	}
	foundTarget := false
	for _, member := range team.Members {
		if member.RoleID == roleID {
			foundTarget = true
			continue
		}
		if !roleExists(global.Roles, member.RoleID) && !roleExists(project.Roles, member.RoleID) {
			return ProjectRegistries{}, fmt.Errorf("%w: 事务恢复不能猜测其他成员引用", ErrRoleCorrupt)
		}
	}
	if !foundTarget {
		return ProjectRegistries{}, fmt.Errorf("%w: 事务恢复不能猜测其他成员引用", ErrRoleCorrupt)
	}
	return ProjectRegistries{Global: global, Project: project, Team: team}, nil
}

func findRole(roles []Role, id string) (int, Role) {
	for index, role := range roles {
		if role.ID == id {
			return index, role
		}
	}
	return -1, Role{}
}

func roleExists(roles []Role, id string) bool { _, found := findRole(roles, id); return found.ID != "" }

func removeRole(roles []Role, id string) []Role {
	index, _ := findRole(roles, id)
	if index < 0 {
		return roles
	}
	return append(roles[:index], roles[index+1:]...)
}

func memberInScope(team TeamRegistry, roleID, scope string) bool {
	for _, member := range team.Members {
		if member.RoleID == roleID {
			return member.RoleScope == scope
		}
	}
	return false
}
