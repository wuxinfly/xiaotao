import * as THREE from 'three';

const target = new THREE.Vector3(0, 1.45, 0);
const offset = new THREE.Vector3(10, 13, -17);
const roomCorners = [
  [-7.2, -.5, -5.4], [7.2, -.5, -5.4], [-7.2, -.5, 5.3], [7.2, -.5, 5.3],
  [-7.2, 3.95, 5.3], [7.2, 3.95, 5.3], [-7.2, 2.55, -2.4],
] as const;

export function fitCamera(camera: THREE.OrthographicCamera, width: number, height: number, extension: number) {
  const fitted = new THREE.Box3();
  const view = new THREE.Matrix4().lookAt(offset, new THREE.Vector3(), camera.up).invert();
  for (const [x, y, z] of roomCorners) fitted.expandByPoint(new THREE.Vector3(x, y - target.y, z + extension / 2).applyMatrix4(view));
  if (extension) for (const x of [-7.2, 7.2]) fitted.expandByPoint(new THREE.Vector3(x, -.5 - target.y, -5.4 - extension / 2).applyMatrix4(view));
  const aspect = Math.max(.01, width / Math.max(1, height));
  const span = Math.max(fitted.max.y, -fitted.min.y, fitted.max.x / aspect, -fitted.min.x / aspect) * 1.06;
  Object.assign(camera, {left: -span * aspect, right: span * aspect, top: span, bottom: -span});
  camera.updateProjectionMatrix();
}
export function resetCamera(camera: THREE.OrthographicCamera, extension: number) {
  const center = target.clone();
  center.z -= extension / 2;
  camera.position.copy(center).addScaledVector(offset, 1 + extension / 25);
  camera.lookAt(center);
  camera.far = 85 + extension * 3;
  camera.zoom = 1;
  camera.updateMatrixWorld();
  camera.updateProjectionMatrix();
}
export function createCamera(width: number, height: number) {
  const camera = new THREE.OrthographicCamera(-1, 1, 1, -1, .1, 85);
  resetCamera(camera, 0);
  fitCamera(camera, width, height, 0);
  return camera;
}

export function statusPose(status: string) {
  return ({running: '工作中', waiting: '待确认', queued: '排队中', failed: '需关注', idle: '空闲'} as Record<string, string>)[status] || status;
}
