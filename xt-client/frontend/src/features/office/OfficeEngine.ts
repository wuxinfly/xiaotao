import * as THREE from 'three';
import {createCamera, placeCharacter, statusPose} from './scene-geometry';
import {sceneLayout, type SceneMember} from './scene-contract';
import {instantiateCharacter, type OfficeAssets} from './scene-models';

type Callbacks = {onSelect: (id: string) => void; onShelf: () => void; onError: (message: string) => void};

export class OfficeEngine {
  private renderer: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera: THREE.OrthographicCamera;
  private observer: ResizeObserver;
  private actors = new Map<string, THREE.Group>();
  private shelf: THREE.Group;
  private characterTemplate: THREE.Group;
  private lost = false;
  private pausedMember = '';
  private frame = 0;
  private last = 0;
  private elapsed = 0;
  private hidden = document.hidden;
  private reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  private dragging = false;
  private dragMoved = false;
  private previous = {x: 0, y: 0};
  private start = {x: 0, y: 0};
  private target = new THREE.Vector3(0, 1, .7);
  private motionQuery = matchMedia('(prefers-reduced-motion: reduce)');
  private active = true;
  private enabled = true;
  private intersection: IntersectionObserver;
  private destroyed = false;

  constructor(private element: HTMLElement, private callbacks: Callbacks, assets: OfficeAssets) {
    this.renderer = new THREE.WebGLRenderer({antialias: true, powerPreference: 'high-performance'});
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFShadowMap;
    this.renderer.domElement.dataset.officeRenderer = 'three';
    this.renderer.domElement.setAttribute('aria-hidden', 'true');
    this.renderer.domElement.addEventListener('webglcontextlost', this.contextLost);
    element.append(this.renderer.domElement);
    this.scene.background = new THREE.Color('#e4e0d7');
    this.scene.fog = new THREE.Fog('#e4e0d7', 24, 40);
    this.camera = createCamera(element.clientWidth, element.clientHeight);
    this.scene.add(new THREE.HemisphereLight('#fff4dc', '#667986', 2.2));
    const sun = new THREE.DirectionalLight('#fff2d3', 3);
    sun.position.set(-5, 10, -4); sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024); sun.shadow.normalBias = .03;
    Object.assign(sun.shadow.camera, {left: -10, right: 10, top: 10, bottom: -10});
    this.scene.add(sun);
    this.shelf = assets.room;
    this.scene.add(this.shelf);
    this.characterTemplate = assets.character;
    this.observer = new ResizeObserver(this.resize);
    this.observer.observe(element);
    this.intersection = new IntersectionObserver(entries => {
      this.active = entries[0]?.isIntersecting ?? true;
      if (this.active) this.schedule(); else this.stop();
    }, {threshold: .01});
    this.intersection.observe(element);
    this.active = element.getBoundingClientRect().bottom > 0;
    element.addEventListener('wheel', this.wheel, {passive: false});
    element.addEventListener('pointerdown', this.pointerDown);
    element.addEventListener('pointermove', this.pointerMove);
    element.addEventListener('pointerup', this.pointerUp);
    element.addEventListener('pointercancel', this.pointerCancel);
    document.addEventListener('visibilitychange', this.visibility);
    this.motionQuery.addEventListener('change', this.motion);
    this.resize(); this.schedule();
  }

  update(members: SceneMember[]) {
    const layouts = sceneLayout(members);
    for (const [id, actor] of this.actors) {
      if (layouts.some(item => item.id === id)) continue;
      this.scene.remove(actor); this.dispose(actor); this.actors.delete(id);
    }
    for (const layout of layouts) {
      const member = members.find(item => item.id === layout.id);
      if (!member) continue;
      const actor = this.actors.get(layout.id);
      if (actor && JSON.stringify(actor.userData.appearance) === JSON.stringify(member.appearance)) {
        actor.userData.status = member.status;
        placeCharacter(actor, layout);
        this.updateLabel(actor, member); continue;
      }
      if (actor) {this.scene.remove(actor); this.dispose(actor);}
      const next = instantiateCharacter(this.characterTemplate, member.appearance, member.id);
      placeCharacter(next, layout); next.userData.status = member.status;
      next.userData.appearance = member.appearance;
      this.updateLabel(next, member);
      this.scene.add(next); this.actors.set(member.id, next);
    }
    this.render();
  }

  private updateLabel(actor: THREE.Group, member: SceneMember) {
    const signature = `${member.name}|${member.status}|${member.responsibility || member.role || ''}`;
    if (actor.userData.label === signature) return;
    actor.userData.label = signature;
    const canvas = document.createElement('canvas'); canvas.width = 512; canvas.height = 128;
    const context = canvas.getContext('2d'); if (!context) return;
    context.fillStyle = '#263b45'; context.beginPath(); context.roundRect(36, 18, 440, 92, 18); context.fill();
    context.fillStyle = '#f8f6ef'; context.font = 'bold 32px sans-serif'; context.textAlign = 'center';
    context.fillText(member.name, 256, 59);
    context.fillStyle = member.status === 'failed' ? '#ffc3b0' : member.status === 'running' ? '#b3f0d6' : '#d7e2e4';
    context.font = '23px sans-serif'; context.fillText(`${member.responsibility || member.role || '项目成员'} · ${statusPose(member.status)}`, 256, 93);
    const old = actor.getObjectByName('member-label') as THREE.Sprite | undefined;
    if (old) {old.material.map?.dispose(); old.material.dispose(); actor.remove(old);}
    const label = new THREE.Sprite(new THREE.SpriteMaterial({map: new THREE.CanvasTexture(canvas), depthTest: false}));
    label.name = 'member-label'; label.position.set(0, 3.04, 0); label.scale.set(2.25, .5625, 1);
    actor.updateMatrixWorld(true);
    label.position.applyMatrix4(actor.matrixWorld.clone().invert());
    actor.getWorldScale(label.scale);
    label.scale.set(2.25 / label.scale.x, .5625 / label.scale.y, 1);
    actor.add(label);
  }

  private resize = () => {
    const width = this.element.clientWidth;
    const height = this.element.clientHeight;
    if (width <= 0 || height <= 0) {this.stop(); return;}
    const aspect = width / height;
    this.camera.left = -6.6 * aspect; this.camera.right = 6.6 * aspect;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(width, height);
    this.render();
  };
  private visibility = () => {this.hidden = document.hidden; this.updateActivity();};
  private motion = (event: MediaQueryListEvent) => {this.reduced = event.matches; this.render();};
  private updateActivity() {if (this.hidden || !this.active || !this.enabled) this.stop(); else this.schedule();}
  private schedule() {if (!this.destroyed && !this.hidden && this.active && this.enabled && !this.frame && this.element.clientWidth > 0) this.frame = requestAnimationFrame(this.animate);}
  private stop() {cancelAnimationFrame(this.frame); this.frame = 0;}
  render() {if (!this.destroyed && !this.lost && this.element.clientWidth > 0) {try {this.renderer.render(this.scene, this.camera);} catch {this.contextLost();}}}
  private contextLost = (event?: Event) => {event?.preventDefault(); if (this.lost) return; this.lost = true; this.stop(); this.callbacks.onError('图形上下文已中断。请重新加载场景，或继续使用二维团队列表。');};
  private animate = (time: number) => {
    this.frame = 0;
    if (this.hidden || !this.active || !this.enabled || this.lost) return;
    if (time - this.last >= 1000 / 30) {
      this.elapsed += Math.min(50, time - this.last); this.last = time;
      if (this.reduced) {this.render(); return;}
      this.actors.forEach((actor, id) => statusMotion(actor, this.elapsed, id === this.pausedMember));
      this.renderer.render(this.scene, this.camera);
    }
    this.schedule();
  };
  private wheel = (event: WheelEvent) => {event.preventDefault(); this.zoomBy(event.deltaY < 0 ? .08 : -.08);};
  private pointerDown = (event: PointerEvent) => {
    if (event.button !== 0) return;
    this.dragging = true; this.dragMoved = false; this.start = {x: event.clientX, y: event.clientY}; this.previous = {...this.start};
    this.element.setPointerCapture(event.pointerId);
  };
  private pointerMove = (event: PointerEvent) => {
    if (!this.dragging) return;
    const scale = 10 / Math.max(1, this.element.clientHeight) / this.camera.zoom;
    this.target.x = THREE.MathUtils.clamp(this.target.x - (event.clientX - this.previous.x) * scale, -3, 3);
    const forward = new THREE.Vector3(); this.camera.getWorldDirection(forward); forward.y = 0; forward.normalize();
    const right = new THREE.Vector3().crossVectors(forward, this.camera.up).normalize();
    this.target.addScaledVector(right, -(event.clientX - this.previous.x) * scale);
    this.target.addScaledVector(forward, (event.clientY - this.previous.y) * scale);
    this.target.x = THREE.MathUtils.clamp(this.target.x, -3, 3);
    this.target.z = THREE.MathUtils.clamp(this.target.z, -2, 3);
    if (Math.hypot(event.clientX - this.start.x, event.clientY - this.start.y) > 5) this.dragMoved = true;
    this.camera.lookAt(this.target); this.previous = {x: event.clientX, y: event.clientY};
    this.render();
  };
  private pointerUp = (event: PointerEvent) => {
    if (!this.dragging) return;
    this.dragging = false;
    if (this.element.hasPointerCapture(event.pointerId)) this.element.releasePointerCapture(event.pointerId);
    if (this.dragMoved) return;
    const bounds = this.renderer.domElement.getBoundingClientRect();
    const pointer = new THREE.Vector2((event.clientX - bounds.left) / bounds.width * 2 - 1, -(event.clientY - bounds.top) / bounds.height * 2 + 1);
    const raycaster = new THREE.Raycaster(); raycaster.setFromCamera(pointer, this.camera);
    const hit = raycaster.intersectObjects([...this.actors.values(), this.shelf], true)[0];
    const id = hit?.object.userData.memberId;
    if (id) this.callbacks.onSelect(id);
    else if (hit?.object.userData.action === 'artifacts') this.callbacks.onShelf();
  };
  private pointerCancel = () => {this.dragging = false;};
  pauseMember(id: string) {this.pausedMember = id;}
  zoomBy(amount: number) {this.camera.zoom = THREE.MathUtils.clamp(this.camera.zoom + amount, .75, 1.6); this.camera.updateProjectionMatrix(); this.render();}
  reset() {this.target.set(0, 1, .7); this.camera.position.set(10, 11, -14); this.camera.zoom = 1; this.camera.lookAt(this.target); this.camera.updateProjectionMatrix(); this.render();}
  setActive(active: boolean) {this.enabled = active; this.updateActivity(); if (active) this.render();}
  private dispose(object: THREE.Object3D) {
    object.traverse(child => {
      if (!(child instanceof THREE.Mesh) && !(child instanceof THREE.Sprite)) return;
      if ('geometry' in child) child.geometry.dispose();
      const materials = Array.isArray(child.material) ? child.material : [child.material];
      for (const material of materials) {material.map?.dispose(); material.dispose();}
    });
  }
  destroy() {
    this.destroyed = true;
    this.stop(); this.observer.disconnect();
    this.intersection.disconnect();
    document.removeEventListener('visibilitychange', this.visibility);
    this.motionQuery.removeEventListener('change', this.motion);
    this.element.removeEventListener('wheel', this.wheel); this.element.removeEventListener('pointerdown', this.pointerDown);
    this.element.removeEventListener('pointermove', this.pointerMove); this.element.removeEventListener('pointerup', this.pointerUp);
    this.element.removeEventListener('pointercancel', this.pointerCancel);
    this.renderer.domElement.removeEventListener('webglcontextlost', this.contextLost);
    this.dispose(this.scene); this.renderer.dispose(); this.renderer.domElement.remove();
  }
}

function statusMotion(actor: THREE.Group, time: number, paused: boolean) {
  if (paused) return;
  actor.rotation.x = actor.userData.status === 'failed' ? -.05 : 0;
  actor.position.y = actor.userData.status === 'running' ? Math.abs(Math.sin(time * .003)) * .025 : 0;
}
