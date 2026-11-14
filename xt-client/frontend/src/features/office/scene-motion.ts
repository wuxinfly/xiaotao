import * as THREE from 'three';
import {mustBeSeated, type SceneMember} from './scene-contract';
import type {instantiateCharacter} from './scene-models';
import {aisleX, seatOffset, type Workstation} from './scene-workstations';

type PosedCharacter = ReturnType<typeof instantiateCharacter>;
const seatedAngle = Math.PI * .85;
const floor = .035;

function delay(id: string) {
  let hash = 0;
  for (const char of id) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) >>> 0;
  return 1.8 + hash % 4200 / 1000;
}

export class OfficeMotion {
  private seat = 1;
  private route: THREE.Vector2[] = [];
  private nextWalk: number;
  private locked: boolean;
  private working = false;
  private stride = 0;
  private readonly home: THREE.Vector2;
  roaming = false;

  constructor(private character: PosedCharacter, member: SceneMember, private desk: Workstation, time: number) {
    this.home = new THREE.Vector2(desk.x, desk.z - seatOffset);
    this.nextWalk = time + delay(member.id);
    this.locked = mustBeSeated(member);
    this.working = member.status === 'running';
    this.snapHome();
  }

  update(member: SceneMember, time: number) {
    const locked = mustBeSeated(member);
    if (locked && !this.locked) this.routeHome();
    if (!locked && this.locked) this.nextWalk = time + delay(member.id);
    this.locked = locked;
    const wasWorking = this.working;
    this.working = member.status === 'running';
    if (wasWorking && !this.working && !this.roaming) this.pose(0, false);
  }

  get animating() {return this.roaming || !this.locked || this.working;}
  get walkDue() {return this.locked || this.roaming ? Infinity : this.nextWalk;}

  snapHome() {
    const {actor} = this.character;
    actor.position.set(this.home.x, floor, this.home.y);
    actor.rotation.y = seatedAngle;
    this.route = []; this.roaming = false; this.seat = 1;
    this.pose(0, false);
  }

  step(time: number, dt: number, canRoam: boolean) {
    if (!this.locked && !this.roaming && canRoam && time >= this.nextWalk) this.walk();
    if (!this.roaming && !this.working) return false;
    const target = this.route[0];
    this.seat = THREE.MathUtils.clamp(this.seat + (target ? -1 : 1) * dt * 3.1, 0, 1);
    if (target && this.seat === 0) this.move(target, dt);
    else if (!target) this.turn(seatedAngle, dt);
    if (this.roaming && !this.route.length && this.seat === 1) {
      this.roaming = false;
      this.nextWalk = time + 7;
    }
    this.pose(time, !!target && this.seat === 0);
    return true;
  }

  private walk() {
    const lane = new THREE.Vector2(aisleX, this.home.y);
    this.route = [lane, new THREE.Vector2(aisleX, lane.y + (this.desk.z > 0 ? -1.84 : 1.84)), lane, this.home];
    this.roaming = true;
  }

  private routeHome() {
    if (!this.roaming) return;
    const {position} = this.character.actor;
    this.route = Math.abs(position.z - this.home.y) < .01 ? [this.home] :
      [new THREE.Vector2(aisleX, this.home.y), this.home];
  }

  private move(target: THREE.Vector2, dt: number) {
    const {position} = this.character.actor;
    const x = target.x - position.x, z = target.y - position.z;
    const distance = Math.hypot(x, z);
    if (distance > .001) this.turn(Math.atan2(-x, -z), dt);
    const step = Math.min(distance, dt * 2.05);
    if (distance > 0) {position.x += x * step / distance; position.z += z * step / distance;}
    this.stride += step * 7.2;
    if (distance <= step) this.route.shift();
  }

  private turn(angle: number, dt: number) {
    const rotation = this.character.actor.rotation;
    const delta = angle - rotation.y;
    rotation.y += Math.atan2(Math.sin(delta), Math.cos(delta)) * Math.min(1, dt * 12);
  }

  private pose(time: number, walking: boolean) {
    this.character.actor.position.y = floor - this.seat * .155 +
      (walking ? Math.abs(Math.sin(this.stride)) * .073 : 0);
    this.character.rig.forEach(({thigh, shin, arm}, index) => {
      const cycle = index ? -1 : 1;
      const swing = walking ? Math.sin(this.stride) * cycle : 0;
      if (thigh) thigh.rotation.x = this.seat * 1.26 + swing * .45;
      if (shin) shin.rotation.x = -this.seat * 1.26 - Math.max(0, -swing) * .54;
      if (arm) arm.rotation.x = -swing * .36 + this.seat * (.43 + (this.working ? Math.sin(time * 11 + index * 2) * .11 : 0));
    });
  }
}
