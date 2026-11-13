import * as THREE from 'three';
import type {Appearance} from '../../lib/team';
import type {SceneLayout} from './scene-contract';

const material = (color: string | number) => new THREE.MeshStandardMaterial({color, roughness: .8});
export function shapedHead(shape: Appearance['shape'], color: string | number, id?: string) {
  const geometry = shape === 'square' ? new THREE.BoxGeometry(.72, .72, .66)
    : shape === 'triangle' ? new THREE.ConeGeometry(.48, .82, 3)
    : shape === 'hexagon' ? new THREE.CylinderGeometry(.39, .39, .72, 6)
    : new THREE.SphereGeometry(.36, 16, 12);
  const head = new THREE.Mesh(geometry, material(color)); if (id) head.userData.memberId = id; return head;
}
function box(parent: THREE.Object3D, size: [number, number, number], at: [number, number, number], color: string | number, id?: string) {
  const mesh = new THREE.Mesh(new THREE.BoxGeometry(...size), material(color)); mesh.position.set(...at); mesh.castShadow = true; mesh.receiveShadow = true;
  if (id) mesh.userData.memberId = id; parent.add(mesh); return mesh;
}
export function character(appearance: Appearance, id: string): THREE.Group {
  const group = new THREE.Group(); group.userData.memberId = id;
  const skin = '#e7b792';
  box(group, [.7, .9, .4], [0, 1.45, 0], appearance.primaryColor, id);
  box(group, [.22, .65, .28], [-.19, .65, 0], appearance.secondaryColor, id); box(group, [.22, .65, .28], [.19, .65, 0], appearance.secondaryColor, id);
  box(group, [.26, .16, .43], [-.19, .25, -.05], appearance.secondaryColor, id); box(group, [.26, .16, .43], [.19, .25, -.05], appearance.secondaryColor, id);
  box(group, [.18, .76, .22], [-.48, 1.45, 0], skin, id); box(group, [.18, .76, .22], [.48, 1.45, 0], skin, id);
  const head = shapedHead(appearance.shape, skin, id); head.position.y = 2.2; group.add(head);
  box(group, [.75, .2, .62], [0, 2.47, .02], '#3c2a20', id);
  box(group, [.48, .07, .04], [0, 2.2, -.34], '#263a46', id);
  return group;
}
export function furniture(scene: THREE.Scene) {
  box(scene, [15, .2, 11], [0, -.12, 0], '#bfae96'); box(scene, [15, 5, .2], [0, 2.5, 5.3], '#d9d2c5');
  for (const x of [-3.5, 0, 3.5]) {box(scene, [2.2, .12, 1.1], [x, 1, 3.7], '#866b50'); for (const dx of [-.8, .8]) box(scene, [.12, 1, .12], [x + dx, .5, 3.7], '#5c5144'); box(scene, [.9, .55, .06], [x, 1.45, 3.8], '#334954');}
  box(scene, [3.3, .1, 1.8], [0, .8, .3], '#a38361'); for (const x of [-1.3, 1.3]) box(scene, [.15, .8, .15], [x, .4, .3], '#65533f');
  const shelf = new THREE.Group(); shelf.position.set(6, 0, 4.8); shelf.userData.action = 'artifacts';
  box(shelf, [1.8, 2.5, .35], [0, 1.25, 0], '#80684e'); for (const y of [.5, 1.25, 2]) box(shelf, [1.6, .4, .2], [0, y, -.25], '#638285'); scene.add(shelf);
  box(scene, [2.8, 1.8, .06], [-3.5, 3, 5.1], '#a5c6ce');
  return shelf;
}
export function placeCharacter(group: THREE.Group, layout: SceneLayout) {group.position.set(layout.x, 0, layout.z); group.rotation.y = layout.rotation;}
export function createCamera(width: number, height: number) {
  const aspect = width / Math.max(1, height);
  const camera = new THREE.OrthographicCamera(-6.6 * aspect, 6.6 * aspect, 5, -5, .1, 80);
  camera.position.set(10, 11, -14); camera.lookAt(0, 1, .7); camera.updateMatrixWorld(); return camera;
}
export function statusPose(status: string) {
  return ({running: '工作中', waiting: '等待中', failed: '需关注', idle: '空闲'} as Record<string, string>)[status] || status;
}
export function projectMember(layout: SceneLayout, width: number, height: number) {
  const point = new THREE.Vector3(layout.x, 1.45, layout.z).project(createCamera(width, height));
  return {x: (point.x + 1) * width / 2, y: (1 - point.y) * height / 2};
}
