package office

import "errors"

type RoleRequest struct {
	ProjectPath string            `json:"projectPath"`
	RoleID      string            `json:"roleId,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Draft       RoleDraft         `json:"role"`
	Revisions   ProjectRegistries `json:"revisions"`
}
type TeamRequest struct {
	ProjectPath string            `json:"projectPath"`
	TemplateID  string            `json:"templateId"`
	Roles       []RoleDraft       `json:"roles"`
	Revisions   ProjectRegistries `json:"revisions"`
}

func (s *Service) TeamTemplates() []TeamTemplate                  { return TeamTemplates() }
func (s *Service) TeamSnapshot(path string) (TeamSnapshot, error) { return s.teams.Snapshot(path) }
func (s *Service) BootstrapTeam(request TeamRequest) (TeamSnapshot, error) {
	if request.TemplateID != "" && request.TemplateID != TeamTemplates()[0].ID {
		return TeamSnapshot{}, errors.New("未知团队模板")
	}
	return s.teams.Bootstrap(request.ProjectPath, request.Roles, request.Revisions)
}
func (s *Service) SkipTeamOnboarding(request TeamRequest) (TeamSnapshot, error) {
	return s.teams.SkipOnboarding(request.ProjectPath, request.Revisions)
}
func (s *Service) CreateTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.CreateRole(request.ProjectPath, request.Scope, request.Draft, request.Revisions)
}
func (s *Service) UpdateTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.UpdateRole(request.ProjectPath, request.RoleID, request.Draft, request.Revisions)
}
func (s *Service) InviteTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.Invite(request.ProjectPath, request.RoleID, request.Revisions)
}
func (s *Service) RemoveTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.Remove(request.ProjectPath, request.RoleID, request.Revisions)
}
func (s *Service) PromoteTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.Promote(request.ProjectPath, request.RoleID, request.Revisions)
}
func (s *Service) ArchiveTeamRole(request RoleRequest) (TeamSnapshot, error) {
	return s.teams.Archive(request.ProjectPath, request.RoleID, request.Revisions)
}
