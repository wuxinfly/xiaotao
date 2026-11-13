package office

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrMemberActive = errors.New("成员仍有进行中的对话，请稍后移出")

type TeamService struct {
	store  *RegistryStore
	now    func() time.Time
	active func(string, string) bool
}

func (s *TeamService) SetActiveCheck(check func(string, string) bool) { s.active = check }

type MemberView struct {
	Role
	JoinedAt time.Time `json:"joinedAt"`
}

type TeamSnapshot struct {
	Members         []MemberView `json:"members"`
	Onboarding      string       `json:"onboarding"`
	GlobalRoles     []Role       `json:"globalRoles"`
	ProjectRoles    []Role       `json:"projectRoles"`
	GlobalRevision  int64        `json:"globalRevision"`
	ProjectRevision int64        `json:"projectRevision"`
	TeamRevision    int64        `json:"teamRevision"`
}

func (s *TeamService) Active(path, roleID string) bool {
	snapshot, err := s.Snapshot(path)
	if err != nil {
		return false
	}
	_, err = snapshot.Member(roleID)
	return err == nil
}

func NewTeamService(store *RegistryStore) *TeamService {
	return &TeamService{store: store, now: time.Now}
}

func (s *TeamService) Snapshot(projectPath string) (TeamSnapshot, error) {
	registries, err := s.store.Load(projectPath)
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(registries), nil
}

func snapshotFromRegistries(registries ProjectRegistries) TeamSnapshot {
	members := make([]MemberView, 0, len(registries.Team.Members))
	for _, membership := range registries.Team.Members {
		roles := registries.Project.Roles
		if membership.RoleScope == RoleScopeGlobal {
			roles = registries.Global.Roles
		}
		_, role := findRole(roles, membership.RoleID)
		members = append(members, MemberView{Role: role, JoinedAt: membership.JoinedAt})
	}
	return TeamSnapshot{Members: members, Onboarding: registries.Team.EffectiveOnboarding(), GlobalRoles: activeRoles(registries.Global.Roles), ProjectRoles: registries.Project.Roles, GlobalRevision: registries.Global.Revision, ProjectRevision: registries.Project.Revision, TeamRevision: registries.Team.Revision}
}

func (s *TeamService) SkipOnboarding(projectPath string, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		if len(value.Team.Members) != 0 {
			return errors.New("当前项目已有团队，无需稍后设置")
		}
		value.Team.Onboarding = OnboardingSkipped
		value.Team.Revision++
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) Bootstrap(projectPath string, drafts []RoleDraft, expected ProjectRegistries) (TeamSnapshot, error) {
	if len(drafts) == 0 || len(drafts) > MaxProjectMembers {
		return TeamSnapshot{}, errors.New("建队需要 1 到 32 个角色")
	}
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		if len(value.Team.Members) != 0 {
			return errors.New("当前项目已有团队，请使用邀请或编辑")
		}
		if len(value.Project.Roles)+len(drafts) > MaxRegistryRoles {
			return errors.New("项目角色数量已达上限")
		}
		now := s.now().UTC()
		for index, draft := range drafts {
			role, roleErr := newRole(draft, RoleScopeProject, index, now)
			if roleErr != nil {
				return roleErr
			}
			value.Project.Roles = append(value.Project.Roles, role)
			value.Team.Members = append(value.Team.Members, Membership{RoleID: role.ID, RoleScope: RoleScopeProject, JoinedAt: now})
		}
		value.Project.Revision++
		value.Team.Revision++
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) CreateRole(projectPath, scope string, draft RoleDraft, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		role, err := newRole(draft, scope, len(value.Project.Roles), s.now().UTC())
		if err != nil {
			return err
		}
		if scope == RoleScopeGlobal {
			value.Global.Roles = append(value.Global.Roles, role)
			value.Global.Revision++
		} else if scope == RoleScopeProject {
			value.Project.Roles = append(value.Project.Roles, role)
			value.Project.Revision++
		} else {
			return errors.New("未知角色 scope")
		}
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) UpdateRole(projectPath, roleID string, draft RoleDraft, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		index, role := findRole(value.Project.Roles, roleID)
		registry := &value.Project
		if index < 0 {
			index, role = findRole(value.Global.Roles, roleID)
			registry = &value.Global
		}
		if index < 0 || role.Lifecycle != RoleActive {
			return ErrRoleNotFound
		}
		if err := validateDraft(draft); err != nil {
			return err
		}
		role.Name, role.Responsibility, role.Capabilities, role.Model, role.Appearance = strings.TrimSpace(draft.Name), strings.TrimSpace(draft.Responsibility), append([]string{}, draft.Capabilities...), draft.Model, draft.Appearance
		role.UpdatedAt = s.now().UTC()
		registry.Roles[index] = role
		registry.Revision++
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) Invite(projectPath, roleID string, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		if memberExists(value.Team, roleID) {
			return errors.New("该角色已是当前项目成员")
		}
		index, role := findRole(value.Global.Roles, roleID)
		if index < 0 || role.Lifecycle != RoleActive {
			return ErrRoleNotFound
		}
		value.Team.Members = append(value.Team.Members, Membership{RoleID: roleID, RoleScope: RoleScopeGlobal, JoinedAt: s.now().UTC()})
		value.Team.Revision++
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) Remove(projectPath, roleID string, expected ProjectRegistries) (TeamSnapshot, error) {
	if s.active != nil && s.active(projectPath, roleID) {
		return TeamSnapshot{}, ErrMemberActive
	}
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		for index, member := range value.Team.Members {
			if member.RoleID == roleID {
				value.Team.Members = append(value.Team.Members[:index], value.Team.Members[index+1:]...)
				value.Team.Revision++
				return nil
			}
		}
		return ErrMemberNotFound
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) Archive(projectPath, roleID string, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Update(projectPath, expected, func(value *ProjectRegistries) error {
		if memberExists(value.Team, roleID) {
			return errors.New("请先将角色移出当前项目")
		}
		index, role := findRole(value.Project.Roles, roleID)
		registry := &value.Project
		if index < 0 {
			index, role = findRole(value.Global.Roles, roleID)
			registry = &value.Global
		}
		if index < 0 {
			return ErrRoleNotFound
		}
		if role.Scope == RoleScopeGlobal {
			if s.store.projects == nil {
				return errors.New("无法验证全部项目引用，拒绝归档")
			}
			paths, err := s.store.projects()
			if err != nil {
				return fmt.Errorf("无法验证全部项目引用，拒绝归档: %w", err)
			}
			for _, path := range paths {
				team, err := readTeam(s.store.paths(path).team)
				if err != nil {
					return fmt.Errorf("无法验证全部项目引用，拒绝归档: %w", err)
				}
				if memberExists(team, roleID) {
					return errors.New("角色仍被其他项目成员关系引用")
				}
			}
		}
		role.Lifecycle, role.UpdatedAt = RoleArchived, s.now().UTC()
		registry.Roles[index] = role
		registry.Revision++
		return nil
	})
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func (s *TeamService) Promote(projectPath, roleID string, expected ProjectRegistries) (TeamSnapshot, error) {
	result, err := s.store.Promote(projectPath, roleID, expected, s.now())
	if err != nil {
		return TeamSnapshot{}, err
	}
	return snapshotFromRegistries(result), nil
}

func newRole(draft RoleDraft, scope string, index int, now time.Time) (Role, error) {
	if err := validateDraft(draft); err != nil {
		return Role{}, err
	}
	return Role{ID: "role_" + randomID(8), Scope: scope, Lifecycle: RoleActive, Name: strings.TrimSpace(draft.Name), Responsibility: strings.TrimSpace(draft.Responsibility), Capabilities: append([]string{}, draft.Capabilities...), Model: draft.Model, Appearance: draft.Appearance, CreatedAt: now, UpdatedAt: now}, nil
}

func validateDraft(draft RoleDraft) error {
	role := Role{ID: "role_0123456789abcdef", Scope: RoleScopeProject, Lifecycle: RoleActive, Name: draft.Name, Responsibility: draft.Responsibility, Capabilities: draft.Capabilities, Model: draft.Model, Appearance: draft.Appearance, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return validateRole(role, RoleScopeProject)
}

func activeRoles(roles []Role) []Role {
	result := make([]Role, 0, len(roles))
	for _, role := range roles {
		if role.Lifecycle == RoleActive {
			result = append(result, role)
		}
	}
	return result
}

func memberExists(team TeamRegistry, roleID string) bool {
	for _, member := range team.Members {
		if member.RoleID == roleID {
			return true
		}
	}
	return false
}

func (snapshot TeamSnapshot) Member(roleID string) (MemberView, error) {
	for _, member := range snapshot.Members {
		if member.ID == roleID {
			return member, nil
		}
	}
	return MemberView{}, fmt.Errorf("%w: %s", ErrMemberNotFound, roleID)
}
