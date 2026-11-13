package office

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	RoleSchemaVersion   = 1
	RoleScopeGlobal     = "global"
	RoleScopeProject    = "project"
	RoleActive          = "active"
	RoleStaged          = "staged"
	RoleArchived        = "archived"
	OnboardingPending   = "pending"
	OnboardingSkipped   = "skipped"
	OnboardingCompleted = "completed"
	MaxProjectMembers   = 32
	MaxRegistryRoles    = 128
)

var (
	ErrRoleConflict   = errors.New("角色记录已被修改，请重新加载")
	ErrRoleCorrupt    = errors.New("角色记录损坏")
	ErrRoleNotFound   = errors.New("角色不存在")
	ErrMemberNotFound = errors.New("成员不存在")
	roleIDPattern     = regexp.MustCompile(`^role_[a-f0-9]{16}$`)
	colorPattern      = regexp.MustCompile(`^#[a-f0-9]{6}$`)
	appearanceShapes  = map[string]bool{"round": true, "square": true, "triangle": true, "hexagon": true}
)

type ModelPreference struct {
	ModelID         string `json:"modelId" yaml:"model_id"`
	ContextWindow   int64  `json:"contextWindow,omitempty" yaml:"context_window,omitempty"`
	MaxOutputTokens int64  `json:"maxOutputTokens,omitempty" yaml:"max_output_tokens,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty" yaml:"reasoning_effort,omitempty"`
}

type Appearance struct {
	Shape          string `json:"shape" yaml:"shape"`
	PrimaryColor   string `json:"primaryColor" yaml:"primary_color"`
	SecondaryColor string `json:"secondaryColor" yaml:"secondary_color"`
}

type Role struct {
	ID             string          `json:"id" yaml:"id"`
	Scope          string          `json:"scope" yaml:"scope"`
	Lifecycle      string          `json:"lifecycle" yaml:"lifecycle"`
	Name           string          `json:"name" yaml:"name"`
	Responsibility string          `json:"responsibility" yaml:"responsibility"`
	Capabilities   []string        `json:"capabilities" yaml:"capabilities"`
	Model          ModelPreference `json:"model" yaml:"model"`
	Appearance     Appearance      `json:"appearance" yaml:"appearance"`
	CreatedAt      time.Time       `json:"createdAt" yaml:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" yaml:"updated_at"`
}

type RoleRegistry struct {
	SchemaVersion int    `json:"schemaVersion" yaml:"schema_version"`
	Revision      int64  `json:"revision" yaml:"revision"`
	Roles         []Role `json:"roles" yaml:"roles"`
}

type Membership struct {
	RoleID    string    `json:"roleId" yaml:"role_id"`
	RoleScope string    `json:"roleScope" yaml:"role_scope"`
	JoinedAt  time.Time `json:"joinedAt" yaml:"joined_at"`
}

type TeamRegistry struct {
	SchemaVersion int          `json:"schemaVersion" yaml:"schema_version"`
	Revision      int64        `json:"revision" yaml:"revision"`
	Onboarding    string       `json:"onboarding" yaml:"onboarding"`
	Members       []Membership `json:"members" yaml:"members"`
}

func ValidateRoleRegistry(registry RoleRegistry, scope string) error {
	if registry.SchemaVersion != RoleSchemaVersion {
		return fmt.Errorf("%w: 不支持的 schema_version %d", ErrRoleCorrupt, registry.SchemaVersion)
	}
	if registry.Revision < 0 || scope != RoleScopeGlobal && scope != RoleScopeProject {
		return fmt.Errorf("%w: 无效注册表 revision 或 scope", ErrRoleCorrupt)
	}
	if len(registry.Roles) > MaxRegistryRoles {
		return fmt.Errorf("%w: 角色最多 %d 个", ErrRoleCorrupt, MaxRegistryRoles)
	}
	ids := make(map[string]bool, len(registry.Roles))
	for index, role := range registry.Roles {
		if ids[role.ID] {
			return fmt.Errorf("%w: roles[%d] 重复角色 ID %q", ErrRoleCorrupt, index, role.ID)
		}
		ids[role.ID] = true
		if err := validateRole(role, scope); err != nil {
			return fmt.Errorf("%w: roles[%d]: %v", ErrRoleCorrupt, index, err)
		}
	}
	return nil
}

func validateRole(role Role, scope string) error {
	if !roleIDPattern.MatchString(role.ID) || role.Scope != scope {
		return errors.New("角色 ID 或 scope 无效")
	}
	if role.Lifecycle != RoleActive && role.Lifecycle != RoleStaged && role.Lifecycle != RoleArchived {
		return errors.New("未知角色生命周期")
	}
	if boundedText(role.Name, 1, 40) != nil || boundedText(role.Responsibility, 1, 240) != nil {
		return errors.New("名称或职责为空或超长")
	}
	if role.CreatedAt.IsZero() || role.UpdatedAt.IsZero() || role.UpdatedAt.Before(role.CreatedAt) {
		return errors.New("角色时间无效")
	}
	seen := map[string]bool{}
	for _, tag := range role.Capabilities {
		if len(seen) >= 8 || seen[tag] || boundedText(tag, 1, 24) != nil {
			return errors.New("能力标签重复、超长或超过 8 项")
		}
		seen[tag] = true
	}
	if err := validateModelPreference(role.Model); err != nil {
		return err
	}
	return validateAppearance(role.Appearance)
}

func validateModelPreference(model ModelPreference) error {
	if boundedText(model.ModelID, 0, 200) != nil || boundedText(model.ReasoningEffort, 0, 32) != nil {
		return errors.New("模型偏好超长")
	}
	if model.ContextWindow < 0 || model.MaxOutputTokens < 0 || model.ContextWindow > 0 && model.ContextWindow < 1024 {
		return errors.New("模型上下文参数无效")
	}
	if model.MaxOutputTokens > 0 && model.MaxOutputTokens < 16 || model.ContextWindow > 0 && model.MaxOutputTokens >= model.ContextWindow {
		return errors.New("模型最大输出参数无效")
	}
	return nil
}

func validateAppearance(appearance Appearance) error {
	if !appearanceShapes[appearance.Shape] || !colorPattern.MatchString(appearance.PrimaryColor) || !colorPattern.MatchString(appearance.SecondaryColor) {
		return errors.New("外观造型或配色无效")
	}
	return nil
}

func ValidateTeam(team TeamRegistry, global, project RoleRegistry) error {
	if team.SchemaVersion != RoleSchemaVersion || team.Revision < 0 {
		return fmt.Errorf("%w: 无效成员表版本", ErrRoleCorrupt)
	}
	if team.Onboarding != "" && team.Onboarding != OnboardingPending && team.Onboarding != OnboardingSkipped {
		return fmt.Errorf("%w: 无效引导状态", ErrRoleCorrupt)
	}
	if len(team.Members) > MaxProjectMembers {
		return fmt.Errorf("%w: 项目成员最多 %d 个", ErrRoleCorrupt, MaxProjectMembers)
	}
	roles := map[string]Role{}
	for _, role := range append(append([]Role{}, global.Roles...), project.Roles...) {
		roles[role.ID] = role
	}
	ids := map[string]bool{}
	for index, member := range team.Members {
		role, found := roles[member.RoleID]
		if ids[member.RoleID] || !found || role.Scope != member.RoleScope || role.Lifecycle != RoleActive {
			return fmt.Errorf("%w: members[%d] 引用无效或重复", ErrRoleCorrupt, index)
		}
		ids[member.RoleID] = true
		if member.JoinedAt.IsZero() {
			return fmt.Errorf("%w: members[%d] joined_at 无效", ErrRoleCorrupt, index)
		}
	}
	return nil
}

func (team TeamRegistry) EffectiveOnboarding() string {
	if len(team.Members) > 0 {
		return OnboardingCompleted
	}
	if team.Onboarding == OnboardingSkipped {
		return OnboardingSkipped
	}
	return OnboardingPending
}

func boundedText(value string, min, max int) error {
	length := len([]rune(strings.TrimSpace(value)))
	if length < min || length > max {
		return fmt.Errorf("文本长度须在 %d 到 %d 之间", min, max)
	}
	return nil
}
