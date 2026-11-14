import * as THREE from 'three';
import type {SceneLayout} from './scene-contract';

const target = new THREE.Vector3(0, 1.45, 0);
const offset = new THREE.Vector3(10, 13, -17);
const fitted = new THREE.Box3();
const roomCorners = [
  [-7.2, -.5, -5.4], [7.2, -.5, -5.4], [-7.2, -.5, 5.3], [7.2, -.5, 5.3],
  [-7.2, 3.95, 5.3], [7.2, 3.95, 5.3], [-7.2, 2.55, -2.4],
] as const;

export function fitCamera(camera: THREE.OrthographicCamera, width: number, height: number) {
  const aspect = Math.max(.01, width / Math.max(1, height));
  const span = Math.max(fitted.max.y, -fitted.min.y, fitted.max.x / aspect, -fitted.min.x / aspect) * 1.06;
  Object.assign(camera, {left: -span * aspect, right: span * aspect, top: span, bottom: -span});
  camera.updateProjectionMatrix();
}
export function resetCamera(camera: THREE.OrthographicCamera) {
  camera.position.copy(target).add(offset);
  camera.lookAt(target);
  camera.zoom = 1;
  camera.updateMatrixWorld();
  camera.updateProjectionMatrix();
}
export function createCamera(width: number, height: number) {
  const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, .1, 85);
  resetCamera(camera);
  // Measure the visible model corners in camera space before choosing the frustum.
  fitted.makeEmpty();
  for (const corner of roomCorners) fitted.expandByPoint(new THREE.Vector3(...corner).applyMatrix4(camera.matrixWorldInverse));
  fitCamera(camera, width, height);
  return camera;
}

export function placeCharacter(group: THREE.Group, layout: SceneLayout) {
  group.position.set(layout.x, .04, layout.z);
  group.rotation.y = layout.rotation;
}
export function statusPose(status: string) {
  return ({running: '工作中', waiting: '待确认', failed: '需关注', idle: '空闲'} as Record<string, string>)[status] || status;
}
export function projectMember(layout: SceneLayout, width: number, height: number) {
  const point = new THREE.Vector3(layout.x, 1.43, layout.z).project(createCamera(width, height));
  return {x: (point.x + 1) * width / 2, y: (1 - point.y) * height / 2};
}
