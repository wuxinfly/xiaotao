import * as THREE from 'three';
import {createCamera, fitCamera, placeCharacter, resetCamera, statusPose} from './scene-geometry';
import {sceneLayout, type SceneMember} from './scene-contract';
import {instantiateCharacter, type OfficeAssets} from './scene-models';

type Callbacks = {onSelect: (id: string) => void; onShelf: () => void; onError: (message: string) => void};
const labelOffset = new THREE.Vector3(0, 2.36, 0);

export class OfficeEngine {
  private renderer: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera: THREE.OrthographicCamera;
  private observer: ResizeObserver;
  private actors = new Map<string, ReturnType<typeof instantiateCharacter>>();
  private shelf: THREE.Object3D;
  private room: THREE.Object3D;
  private lost = false;
  private labels = new Map<string, THREE.Sprite>();
  private frame = 0;
  private last = 0;
  private elapsed = 0;
  private animating = false;
  private hidden = document.hidden;
  private reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  private dragging = false;
  private dragMoved = false;
  private previous = {x: 0, y: 0};
  private start = {x: 0, y: 0};
  private pan = new THREE.Vector2();
  private motionQuery = matchMedia('(prefers-reduced-motion: reduce)');
  private active = true;
  private enabled = true;
  private intersection: IntersectionObserver;
  private destroyed = false;

  constructor(private element: HTMLElement, private callbacks: Callbacks, private assets: OfficeAssets) {
    this.renderer = new THREE.WebGLRenderer({alpha: true, antialias: true});
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.toneMappingExposure = 1.12;
    this.renderer.shadowMap.enabled = true;
    this.renderer.shadowMap.type = THREE.PCFShadowMap;
    this.renderer.domElement.dataset.officeRenderer = 'three';
    this.renderer.domElement.setAttribute('aria-hidden', 'true');
    this.renderer.domElement.addEventListener('webglcontextlost', this.contextLost);
    element.append(this.renderer.domElement);
    this.camera = createCamera(element.clientWidth, element.clientHeight);
    this.scene.add(new THREE.HemisphereLight('#edfaff', '#386270', 2.1));
    const sun = new THREE.DirectionalLight('#ffeddb', 2.35);
    sun.position.set(1, 12, -5); sun.castShadow = true;
    sun.shadow.mapSize.set(1024, 1024); sun.shadow.normalBias = .035; sun.shadow.intensity = .52;
    Object.assign(sun.shadow.camera, {left: -10, right: 10, top: 10, bottom: -10});
    this.scene.add(sun);
    this.room = assets.room.clone(true);
    this.shelf = assets.archive.clone(true);
    this.shelf.traverse(object => {object.userData.action = 'artifacts';});
    this.scene.add(this.room, this.shelf);
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
    for (const id of this.actors.keys()) {
      if (layouts.some(item => item.id === id)) continue;
      this.removeActor(id);
    }
    for (const layout of layouts) {
      const member = members.find(item => item.id === layout.id);
      if (!member) continue;
      const actor = this.actors.get(layout.id)?.actor;
      if (actor && JSON.stringify(actor.userData.appearance) === JSON.stringify(member.appearance)) {
        actor.userData.status = member.status;
        placeCharacter(actor, layout);
        statusMotion(actor, this.reduced ? 0 : this.elapsed);
        this.updateLabel(actor, member);
        this.placeLabel(actor, layout.id);
        continue;
      }
      if (actor) this.removeActor(layout.id);
      const next = instantiateCharacter(this.assets, member.appearance, member.id);
      placeCharacter(next.actor, layout); next.actor.userData.status = member.status;
      statusMotion(next.actor, this.reduced ? 0 : this.elapsed);
      this.updateLabel(next.actor, member);
      this.placeLabel(next.actor, member.id);
      this.scene.add(next.actor); this.actors.set(member.id, next);
    }
    this.animating = [...this.actors.values()].some(({actor}) => actor.userData.status === 'running');
    this.updateActivity();
    this.render();
  }

  private updateLabel(actor: THREE.Group, member: SceneMember) {
    const signature = `${member.name}|${member.status}`;
    if (actor.userData.label === signature) return;
    actor.userData.label = signature;
    this.removeLabel(member.id);
    const canvas = document.createElement('canvas'); canvas.width = 400; canvas.height = 98;
    const context = canvas.getContext('2d'); if (!context) return;
    context.fillStyle = '#0d2936'; context.beginPath(); context.roundRect(6, 6, 388, 86, 23); context.fill();
    context.strokeStyle = '#658f9b'; context.lineWidth = 3; context.stroke();
    const tone = member.status === 'failed' ? '#ffafa1' : member.status === 'waiting' ? '#ffd18d' : member.status === 'running' ? '#80f1cb' : '#b0cbd2';
    context.fillStyle = '#f0fcff'; context.font = '600 38px "Microsoft YaHei", sans-serif'; context.textAlign = 'center';
    const name = [...member.name].length > 8 ? `${[...member.name].slice(0, 7).join('')}…` : member.name;
    context.fillText(name, 200, 64, 340);
    context.fillStyle = tone; context.beginPath(); context.arc(31, 49, 8, 0, Math.PI * 2); context.fill();
    const map = new THREE.CanvasTexture(canvas);
    map.colorSpace = THREE.SRGBColorSpace;
    const label = new THREE.Sprite(new THREE.SpriteMaterial({map, depthWrite: false, depthTest: false, toneMapped: false}));
    label.name = `member-label: ${statusPose(member.status)}`;
    label.userData.memberId = member.id;
    label.scale.set(2.24, .55, 1);
    this.scene.add(label);
    this.labels.set(member.id, label);
  }
  private placeLabel(actor: THREE.Group, id: string) {
    this.labels.get(id)?.position.copy(actor.position).add(labelOffset);
  }
  private removeLabel(id: string) {
    const label = this.labels.get(id);
    if (!label) return;
    this.scene.remove(label); label.material.map?.dispose(); label.material.dispose(); this.labels.delete(id);
  }
  private removeActor(id: string) {
    const entry = this.actors.get(id);
    if (!entry) return;
    this.scene.remove(entry.actor); entry.dispose(); this.actors.delete(id); this.removeLabel(id);
  }

  private resize = () => {
    const width = this.element.clientWidth;
    const height = this.element.clientHeight;
    if (width <= 0 || height <= 0) {this.stop(); return;}
    fitCamera(this.camera, width, height);
    this.renderer.setSize(width, height);
    this.render();
  };
  private visibility = () => {this.hidden = document.hidden; this.updateActivity();};
  private motion = (event: MediaQueryListEvent) => {
    this.reduced = event.matches;
    if (this.reduced) {
      this.actors.forEach(({actor}, id) => {statusMotion(actor, 0); this.placeLabel(actor, id);});
      this.stop(); this.render();
    } else this.schedule();
  };
  private updateActivity() {if (this.hidden || !this.active || !this.enabled || !this.animating) this.stop(); else this.schedule();}
  private schedule() {if (!this.destroyed && !this.lost && !this.reduced && !this.hidden && this.active && this.enabled && this.animating && !this.frame && this.element.clientWidth > 0) this.frame = requestAnimationFrame(this.animate);}
  private stop() {cancelAnimationFrame(this.frame); this.frame = 0;}
  render() {if (!this.destroyed && !this.lost && this.element.clientWidth > 0) {try {this.renderer.render(this.scene, this.camera);} catch {this.contextLost();}}}
  private contextLost = (event?: Event) => {event?.preventDefault(); if (this.lost) return; this.lost = true; this.stop(); this.callbacks.onError('图形上下文已中断。请重新加载场景，或继续使用二维团队列表。');};
  private animate = (time: number) => {
    this.frame = 0;
    if (this.hidden || !this.active || !this.enabled || this.lost) return;
    if (time - this.last >= 1000 / 30) {
      this.elapsed += Math.min(50, time - this.last); this.last = time;
      this.actors.forEach(({actor}, id) => {
        statusMotion(actor, this.elapsed);
        this.placeLabel(actor, id);
      });
      this.render();
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
    if (Math.hypot(event.clientX - this.start.x, event.clientY - this.start.y) > 5) this.dragMoved = true;
    if (!this.dragMoved) return;
    const scale = (this.camera.top - this.camera.bottom) / Math.max(1, this.element.clientHeight) / this.camera.zoom;
    const x = THREE.MathUtils.clamp(this.pan.x - (event.clientX - this.previous.x) * scale, -3, 3);
    const y = THREE.MathUtils.clamp(this.pan.y + (event.clientY - this.previous.y) * scale, -2.5, 2.5);
    this.camera.translateX(x - this.pan.x); this.camera.translateY(y - this.pan.y);
    this.pan.set(x, y); this.previous = {x: event.clientX, y: event.clientY};
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
    const labelsHit = raycaster.intersectObjects([...this.labels.values()])[0];
    const hit = raycaster.intersectObjects([this.room, this.shelf, ...Array.from(this.actors.values(), item => item.actor)], true)[0];
    const id = labelsHit?.object.userData.memberId ?? hit?.object.userData.memberId;
    if (id) this.callbacks.onSelect(id);
    else if (hit?.object.userData.action === 'artifacts') this.callbacks.onShelf();
  };
  private pointerCancel = () => {this.dragging = false;};
  zoomBy(amount: number) {this.camera.zoom = THREE.MathUtils.clamp(this.camera.zoom + amount, .75, 1.6); this.camera.updateProjectionMatrix(); this.render();}
  reset() {this.pan.set(0, 0); resetCamera(this.camera); this.render();}
  setActive(active: boolean) {this.enabled = active; this.updateActivity(); if (active) this.render();}
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
    this.actors.forEach((_, id) => this.removeActor(id));
    this.assets.dispose(); this.renderer.dispose(); this.renderer.forceContextLoss(); this.renderer.domElement.remove();
  }
}

function statusMotion(actor: THREE.Group, time: number) {
  actor.rotation.x = actor.userData.status === 'failed' ? -.05 : 0;
  actor.position.y = .04 + (actor.userData.status === 'running' ? Math.abs(Math.sin(time * .003)) * .045 : 0);
}
