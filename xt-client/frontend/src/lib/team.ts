import {request} from './http';

export type RoleScope = 'global' | 'project';
export interface Appearance {
  shape: 'round' | 'square' | 'triangle' | 'hexagon';
  primaryColor: string;
  secondaryColor: string;
}
export const defaultAppearance: Appearance = {shape: 'round', primaryColor: '#36a9c9', secondaryColor: '#173b55'};
export interface ModelPreference {modelId: string; contextWindow: number; maxOutputTokens: number; reasoningEffort?: string}
export interface RoleDraft {name: string; responsibility: string; capabilities: string[]; model: ModelPreference; appearance: Appearance}
export interface Role extends RoleDraft {id: string; scope: RoleScope; lifecycle: string; createdAt: string; updatedAt: string}
export interface TeamMember extends Role {joinedAt: string}
export interface TeamSnapshot {
  members: TeamMember[]; globalRoles: Role[]; projectRoles: Role[];
  onboarding: 'pending' | 'skipped' | 'completed';
  globalRevision: number; projectRevision: number; teamRevision: number;
}
export interface TeamTemplate {id: string; name: string; description: string; roles: RoleDraft[]}
export interface RoleRevisions {global: {revision: number}; project: {revision: number}; team: {revision: number}}

export function roleRevisions(team: TeamSnapshot): RoleRevisions {
  return {global: {revision: team.globalRevision}, project: {revision: team.projectRevision}, team: {revision: team.teamRevision}};
}
function roleRequest(projectPath: string, team: TeamSnapshot, roleId = '', scope: RoleScope = 'project', role?: RoleDraft) {
  const draft = role ? {name: role.name, responsibility: role.responsibility, capabilities: role.capabilities, model: role.model, appearance: role.appearance} : {name: '', responsibility: '', capabilities: [], model: {modelId: '', contextWindow: 0, maxOutputTokens: 0}, appearance: {} as Appearance};
  return {projectPath, roleId, scope, role: draft, revisions: roleRevisions(team)};
}
export const teamApi = {
  templates: (signal?: AbortSignal) => request<TeamTemplate[]>('/api/office/team/templates', {signal}),
  snapshot: (projectPath: string, signal?: AbortSignal) => request<TeamSnapshot>(`/api/office/team?projectPath=${encodeURIComponent(projectPath)}`, {signal}),
  bootstrap: (projectPath: string, team: TeamSnapshot, templateId: string, roles: RoleDraft[]) => request<TeamSnapshot>('/api/office/team/bootstrap', {method: 'POST', body: JSON.stringify({projectPath, templateId, roles: roles.map(({name, responsibility, capabilities, model, appearance}) => ({name, responsibility, capabilities, model, appearance})), revisions: roleRevisions(team)})}),
  skip: (projectPath: string, team: TeamSnapshot) => request<TeamSnapshot>('/api/office/team/skip-onboarding', {method: 'POST', body: JSON.stringify({projectPath, revisions: roleRevisions(team)})}),
  create: (projectPath: string, team: TeamSnapshot, scope: RoleScope, role: RoleDraft) => request<TeamSnapshot>('/api/office/roles', {method: 'POST', body: JSON.stringify(roleRequest(projectPath, team, '', scope, role))}),
  update: (projectPath: string, team: TeamSnapshot, id: string, role: RoleDraft) => request<TeamSnapshot>('/api/office/roles', {method: 'PUT', body: JSON.stringify(roleRequest(projectPath, team, id, 'project', role))}),
  invite: (projectPath: string, team: TeamSnapshot, id: string) => request<TeamSnapshot>('/api/office/roles/invite', {method: 'POST', body: JSON.stringify(roleRequest(projectPath, team, id))}),
  remove: (projectPath: string, team: TeamSnapshot, id: string) => request<TeamSnapshot>('/api/office/roles/remove', {method: 'POST', body: JSON.stringify(roleRequest(projectPath, team, id))}),
  promote: (projectPath: string, team: TeamSnapshot, id: string) => request<TeamSnapshot>('/api/office/roles/promote', {method: 'POST', body: JSON.stringify(roleRequest(projectPath, team, id))}),
  archive: (projectPath: string, team: TeamSnapshot, id: string) => request<TeamSnapshot>('/api/office/roles/archive', {method: 'POST', body: JSON.stringify(roleRequest(projectPath, team, id))}),
};
