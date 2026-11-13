import {useEffect, useRef, useState} from 'react';
import * as THREE from 'three';
import type {Appearance} from '../../lib/team';
import {instantiateCharacter, loadOfficeAssets, type OfficeAssets} from './scene-models';

export function RolePreview({appearance, name}: {appearance: Appearance; name: string}) {
  const host = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (!host.current) return;
    let renderer: THREE.WebGLRenderer;
    let assets: OfficeAssets;
    let disposed = false;
    setFailed(false);
    try {renderer = new THREE.WebGLRenderer({antialias: true, alpha: true});}
    catch {setFailed(true); return;}
    const element = host.current;
    renderer.setSize(180, 220);
    renderer.domElement.setAttribute('aria-hidden', 'true');
    element.replaceChildren(renderer.domElement);
    const scene = new THREE.Scene();
    const camera = new THREE.OrthographicCamera(-2, 2, 2.4, -2.4, .1, 20);
    camera.position.set(3, 2.3, -6); camera.lookAt(0, 1.35, 0);
    scene.add(new THREE.HemisphereLight('#fff4dc', '#667986', 2.2));
    const light = new THREE.DirectionalLight('#fff2d3', 2); light.position.set(-3, 5, -4); scene.add(light);
    loadOfficeAssets().then(loaded => {
      if (disposed) return;
      assets = loaded;
      const actor = instantiateCharacter(assets.character, appearance, 'preview');
      actor.rotation.y = -.45;
      scene.add(actor);
      renderer.render(scene, camera);
    }).catch(() => {if (!disposed) setFailed(true);});
    return () => {disposed = true; if (assets) {scene.traverse(object => {if (object instanceof THREE.Mesh) {object.geometry.dispose(); const materials = Array.isArray(object.material) ? object.material : [object.material]; materials.forEach(material => material.dispose());}});} renderer.dispose(); renderer.domElement.remove();};
  }, [appearance]);
  return <div className="role-preview" ref={host} role="img" aria-label={`${name}外观预览`}>{failed && <span>当前环境无法显示 3D 预览，造型和配色选择仍会保存。</span>}</div>;
}
