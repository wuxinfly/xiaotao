import * as THREE from 'three';
import type {SceneMember} from './scene-contract';
import type {OfficeAssets} from './scene-models';

export const aisleX = 2.72;
export const seatOffset = 1.03;
const rowSpacing = 3.30;

export type Workstation = {x: number; z: number; coordinator: boolean};

export class OfficeWorkstations {
  readonly furniture = new THREE.Group();
  private slots = new Map<string, number>();
  private desks = new Map<string, {station: Workstation; model: THREE.Group}>();
  private extensions: THREE.Object3D[] = [];

  constructor(private assets: OfficeAssets) {}

  update(members: SceneMember[]) {
    const ids = new Set(members.map(member => member.id));
    for (const [id, entry] of this.desks) {
      if (ids.has(id)) continue;
      this.furniture.remove(entry.model);
      this.desks.delete(id);
      this.slots.delete(id);
    }
    const occupied = new Set(this.slots.values());
    let slot = 0;
    for (const member of members) {
      if (this.desks.has(member.id)) continue;
      const coordinator = member.role === 'coordinator';
      while (occupied.has(slot) && !coordinator) slot++;
      const station = coordinator ? {coordinator, x: -3.17, z: 3.62} :
        {coordinator, x: slot % 2 ? 4.64 : .78, z: 2.45 - Math.floor(slot / 2) * rowSpacing};
      if (!coordinator) {this.slots.set(member.id, slot); occupied.add(slot);}
      this.addDesk(member.id, station);
    }
    const count = Math.max(0, ...Array.from(this.slots.values(), index => Math.floor(index / 2) - 1));
    this.extendFloor(count);
    return count * rowSpacing;
  }

  get(id: string) {return this.desks.get(id)?.station;}

  private addDesk(id: string, station: Workstation) {
    const model = new THREE.Group();
    model.position.set(station.x, .035, station.z);
    const desk = (station.coordinator ? this.assets.coordinatorDesk : this.assets.desk).clone();
    const chair = this.assets.chair.clone();
    chair.position.z = -seatOffset;
    model.add(desk, chair);
    model.traverse(object => {object.userData.memberId = id;});
    this.desks.set(id, {station, model});
    this.furniture.add(model);
  }

  private extendFloor(count: number) {
    while (this.extensions.length > count) {
      const tile = this.extensions.pop();
      if (tile) this.furniture.remove(tile);
    }
    while (this.extensions.length < count) {
      const tile = this.assets.floorExtension.clone();
      tile.position.z = -5.29 - (this.extensions.length + .5) * rowSpacing;
      this.extensions.push(tile);
      this.furniture.add(tile);
    }
  }
}
