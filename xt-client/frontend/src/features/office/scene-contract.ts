import type {Appearance} from '../../lib/team';
import type {OfficeMember} from '../../lib/office';
const defaultAppearance: Appearance = {shape: 'round', primaryColor: '#36a9c9', secondaryColor: '#173b55'};
export type SceneMember = {id: string; name: string; status: string; appearance: Appearance; role?: string; responsibility?: string};
export type SceneLayout = {id: string; x: number; z: number; rotation: number};
export function sceneMembers(members: OfficeMember[]): SceneMember[] {
  return members.map(member => ({id: member.id, name: member.name, role: member.role, status: member.status, responsibility: member.responsibility, appearance: member.appearance || defaultAppearance}));
}
export function sceneLayout(members: SceneMember[]): SceneLayout[] {
  const slots = [[-4, 1.5], [-.7, 1.5], [2.7, 2.6], [-4, -1.3], [-.7, -1.3], [2.7, -1.3], [-4, -3.4], [-.7, -3.4], [2.7, -3.4]];
  return members.map((member, index) => ({id: member.id, x: slots[index % slots.length][0], z: slots[index % slots.length][1], rotation: 0}));
}
