import * as THREE from 'three';
import {GLTFLoader} from 'three/addons/loaders/GLTFLoader.js';
import type {Appearance} from '../../lib/team';

export type OfficeAssets = {room: THREE.Object3D; archive: THREE.Object3D; characters: Record<Appearance['shape'], THREE.Object3D>; dispose: () => void};
export async function loadOfficeAssets(signal: AbortSignal): Promise<OfficeAssets> {
  const response = await fetch('/models/office.glb', {signal});
  if (!response.ok) throw new Error(`无法读取本地办公室模型（${response.status}）`);
  const gltf = await new GLTFLoader().parseAsync(await response.arrayBuffer(), '');
  const dispose = () => gltf.scene.traverse(object => {
    if (!(object instanceof THREE.Mesh)) return;
    object.geometry.dispose();
    const materials = Array.isArray(object.material) ? object.material : [object.material];
    for (const material of materials) material.dispose();
  });
  try {
    signal.throwIfAborted();
    function node(name: string) {
      const object = gltf.scene.getObjectByName(name);
      if (!object) throw new Error('本地办公室模型缺少所需节点');
      return object;
    }
    const room = node('OfficeRoom'), archive = node('OfficeArchive');
    const characters = {round: node('QCharacter'), square: node('QCharacter-square'), hexagon: node('QCharacter-hexagon'), triangle: node('QCharacter-triangle')};
    for (const part of [room, archive]) part.traverse(object => {
      if (object instanceof THREE.Mesh) {object.receiveShadow = true; object.castShadow = true;}
    });
    // Keep the asset scene as the owner so it can dispose all shared resources.
    return {room, archive, characters, dispose};
  } catch (error) {dispose(); throw error;}
}

export function instantiateCharacter(assets: OfficeAssets, appearance: Appearance, id: string) {
  const actor = new THREE.Group();
  const model = (assets.characters[appearance.shape] || assets.characters.round).clone(true);
  const primary = new THREE.MeshStandardMaterial({color: appearance.primaryColor || '#36a9c9', roughness: .62});
  const secondary = new THREE.MeshStandardMaterial({color: appearance.secondaryColor || '#173b55', roughness: .77});
  model.traverse(object => {
    object.userData.memberId = id;
    if (!(object instanceof THREE.Mesh)) return;
    if (object.material.name === 'primary') object.material = primary;
    if (object.material.name === 'secondary') object.material = secondary;
    object.castShadow = true;
  });
  actor.userData = {memberId: id, appearance};
  actor.add(model);
  return {actor, dispose: () => {primary.dispose(); secondary.dispose();}};
}
