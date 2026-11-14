import {useEffect, useRef, useState} from 'react';
import * as THREE from 'three';
import type {Appearance} from '../../lib/team';
import {instantiateCharacter, loadOfficeAssets} from './scene-models';

export function RolePreview({appearance, name}: {appearance: Appearance; name: string}) {
  const host = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (!host.current) return;
    let renderer: THREE.WebGLRenderer;
    const controller = new AbortController();
    let disposeModel: (() => void) | undefined;
    setFailed(false);
    try {renderer = new THREE.WebGLRenderer({antialias: true, alpha: true});}
    catch {setFailed(true); return;}
    const element = host.current;
    renderer.setSize(180, 200);
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.domElement.setAttribute('aria-hidden', 'true');
    element.replaceChildren(renderer.domElement);
    const scene = new THREE.Scene();
    const camera = new THREE.OrthographicCamera(-1.2, 1.2, 1.34, -1.34, .1, 20);
    camera.position.set(2.5, 2.2, -6); camera.lookAt(0, 1.04, 0);
    scene.add(new THREE.HemisphereLight('#edfaff', '#386270', 2.1));
    const light = new THREE.DirectionalLight('#ffeddb', 2.35); light.position.set(1, 5, -4); scene.add(light);
    loadOfficeAssets(controller.signal).then(assets => {
      if (controller.signal.aborted) {assets.dispose(); return;}
      const {actor, dispose} = instantiateCharacter(assets, appearance, 'preview');
      disposeModel = () => {dispose(); assets.dispose();};
      actor.rotation.y = -.14;
      scene.add(actor);
      renderer.render(scene, camera);
    }).catch(() => {if (!controller.signal.aborted) setFailed(true);});
    return () => {controller.abort(); disposeModel?.(); renderer.dispose(); renderer.forceContextLoss(); renderer.domElement.remove();};
  }, [appearance]);
  return <div className="role-preview" ref={host} role="img" aria-label={`${name}外观预览`}>{failed && <span>当前环境无法显示 3D 预览，造型和配色选择仍会保存。</span>}</div>;
}
