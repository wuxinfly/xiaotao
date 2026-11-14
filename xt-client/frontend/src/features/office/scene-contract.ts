import type {Appearance} from '../../lib/team';
import type {OfficeMember} from '../../lib/office';
const defaultAppearance: Appearance = {shape: 'round', primaryColor: '#36a9c9', secondaryColor: '#173b55'};
export type SceneMember = {id: string; name: string; status: string; appearance: Appearance; role?: string; taskId?: string; taskStatus?: OfficeMember['taskStatus']};
export function sceneMembers(members: OfficeMember[]): SceneMember[] {
  return members.map(member => ({id: member.id, name: member.name, role: member.role, status: member.status, taskId: member.taskId, taskStatus: member.taskStatus, appearance: member.appearance || defaultAppearance}));
}

export function mustBeSeated(member: SceneMember) {
  return member.role === 'coordinator' || member.role === 'executor' || !!member.taskId ||
    member.status === 'running' || member.status === 'waiting';
}
