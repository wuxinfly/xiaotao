import * as THREE from 'three';

export class OfficeLighting {
  private ambient = new THREE.HemisphereLight();
  private sun = new THREE.DirectionalLight();
  private warm = new THREE.PointLight('#ffc190', 33, 10, 2);
  private panels = new Map<string, THREE.Mesh<THREE.BufferGeometry, THREE.MeshStandardMaterial>[]>();
  private lantern: THREE.MeshStandardMaterial | undefined;
  private day = false;

  constructor(scene: THREE.Scene, room: THREE.Object3D, desk: THREE.Object3D) {
    this.sun.castShadow = true;
    this.sun.shadow.mapSize.set(1024, 1024);
    this.sun.shadow.normalBias = .035;
    this.warm.position.set(-2.1, 3.1, 2.4);
    scene.add(this.ambient, this.sun, this.sun.target, this.warm);
    room.traverse(object => {
      if (!(object instanceof THREE.Mesh && object.material instanceof THREE.MeshStandardMaterial)) return;
      const list = this.panels.get(object.material.name) ?? [];
      list.push(object); this.panels.set(object.material.name, list);
    });
    desk.traverse(object => {
      if (object instanceof THREE.Mesh && object.material instanceof THREE.MeshStandardMaterial && object.material.name === 'lamp') this.lantern = object.material;
    });
  }

  setDay(day: boolean, extension: number) {
    this.day = day;
    this.ambient.color.set(day ? '#e4faff' : '#819ddf');
    this.ambient.groundColor.set(day ? '#77928b' : '#41587f');
    this.ambient.intensity = day ? 2.2 : 1.62;
    this.sun.color.set(day ? '#fff0d6' : '#b3ccff');
    this.sun.intensity = day ? 2.5 : 1.15;
    this.sun.shadow.intensity = day ? .48 : .31;
    this.warm.visible = !day;
    this.panel('window', day ? '#73cdd8' : '#173055', day ? '#377a85' : '#152147');
    this.panel('horizon', day ? '#318b9e' : '#142942', day ? '#1b5064' : '#101a34');
    this.panels.get('sunDisc')?.forEach(mesh => {mesh.visible = day;});
    for (const name of ['nightDisc', 'nightStars']) this.panels.get(name)?.forEach(mesh => {mesh.visible = !day;});
    if (this.lantern) this.lantern.emissiveIntensity = day ? .28 : 1.85;
    this.fit(extension);
  }

  fit(extension: number) {
    const span = 10 + extension / 2;
    this.sun.position.set(this.day ? -5 : 6, 14 + extension / 2, 2 - extension / 2);
    this.sun.target.position.z = -extension / 2;
    const shadow = this.sun.shadow.camera;
    Object.assign(shadow, {left: -span, right: span, top: span, bottom: -span, far: 50 + extension});
    shadow.updateProjectionMatrix();
  }

  private panel(name: string, color: string, glow: string) {
    this.panels.get(name)?.forEach(({material}) => {
      material.color.set(color);
      material.emissive.set(glow);
    });
  }

  dispose() {this.sun.shadow.dispose();}
}
