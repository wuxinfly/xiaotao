import type {Appearance} from '../../lib/team';
import type {OfficeMember} from '../../lib/office';
const defaultAppearance: Appearance = {shape: 'round', primaryColor: '#36a9c9', secondaryColor: '#173b55'};
export type SceneMember = {id: string; name: string; status: string; appearance: Appearance; role?: string; responsibility?: string};
export type SceneLayout = {id: string; x: number; z: number; rotation: number};
export function sceneMembers(members: OfficeMember[]): SceneMember[] {
  return members.map(member => ({id: member.id, name: member.name, role: member.role, status: member.status, responsibility: member.responsibility, appearance: member.appearance || defaultAppearance}));
}
export function sceneLayout(members: SceneMember[]): SceneLayout[] {
  const slots = [[-.74, 1.78], [-4.19, 1.78], [2.71, 1.78], [3.99, -.8], [.36, -.8], [-2.38, -.8], [4.21, -3.25], [1.26, -3.25], [-1.74, -3.25]];
  return members.slice(0, slots.length).map((member, index) => ({id: member.id, x: slots[index][0], z: slots[index][1], rotation: -.20}));
}
