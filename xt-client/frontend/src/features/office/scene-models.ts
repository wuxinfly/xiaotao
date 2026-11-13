import * as THREE from 'three';
import {GLTFLoader} from 'three/addons/loaders/GLTFLoader.js';
import type {Appearance} from '../../lib/team';

export type OfficeAssets = {room: THREE.Group; character: THREE.Group};

export async function loadOfficeAssets(signal?: AbortSignal): Promise<OfficeAssets> {
  const gltf = await new GLTFLoader().loadAsync('/models/office.glb', () => {});
  if (signal?.aborted) {gltf.scene.traverse(dispose); throw new DOMException('Aborted', 'AbortError');}
  const room = gltf.scene.getObjectByName('OfficeRoom');
  const character = gltf.scene.getObjectByName('QCharacter');
  if (!(room instanceof THREE.Group) || !(character instanceof THREE.Group)) throw new Error('本地办公室模型缺少所需节点');
  room.traverse(object => {if (object instanceof THREE.Mesh) {object.receiveShadow = true; object.castShadow = true;}});
  return {room, character};
}

export function instantiateCharacter(template: THREE.Group, appearance: Appearance, id: string) {
  const actor = template.clone(true);
  actor.name = `member-${id}`;
  const clothing = new THREE.MeshStandardMaterial({color: appearance.primaryColor, roughness: .82});
  const bottoms = new THREE.MeshStandardMaterial({color: appearance.secondaryColor, roughness: .86});
  actor.traverse(object => {
    object.userData.memberId = id;
    if (!(object instanceof THREE.Mesh)) return;
    const original = object.material as THREE.MeshStandardMaterial;
    object.material = original.clone();
    const name = (object.material as THREE.MeshStandardMaterial).name;
    if (name === 'clay') {object.material.dispose(); object.material = clothing;}
    if (name === 'navy') {object.material.dispose(); object.material = bottoms;}
    object.castShadow = true;
  });
  actor.userData.appearance = appearance;
  return actor;
}

export function dispose(object: THREE.Object3D) {
  if (!(object instanceof THREE.Mesh) && !(object instanceof THREE.Sprite)) return;
  object.geometry.dispose();
  const materials = Array.isArray(object.material) ? object.material : [object.material];
  for (const material of materials) {material.map?.dispose(); material.dispose();}
}
