import * as THREE from 'three';
import {mustBeSeated, type SceneMember} from './scene-contract';
import type {instantiateCharacter} from './scene-models';
import {aisleX, seatOffset, type Workstation} from './scene-workstations';

type PosedCharacter = ReturnType<typeof instantiateCharacter>;
const seatedAngle = Math.PI;
const floor = .035;
const walkingSpeed = 1.5;
const acceleration = 5;
const turnSpeed = 7;

function angleDelta(from: number, to: number) {
  return Math.atan2(Math.sin(to - from), Math.cos(to - from));
}

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
  private speed = 0;
  private gait = 0;
  private waitUntil = 0;
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
    if (wasWorking && !this.working && !this.roaming) this.pose(0, 0);
  }

  get animating() {return this.roaming || !this.locked || this.working;}
  get walkDue() {return this.locked || this.roaming ? Infinity : this.nextWalk;}

  snapHome() {
    const {actor} = this.character;
    actor.position.set(this.home.x, floor, this.home.y);
    actor.rotation.y = seatedAngle;
    this.route = []; this.roaming = false; this.seat = 1;
    this.speed = this.gait = this.stride = this.waitUntil = 0;
    this.pose(0, 0);
  }

  step(time: number, dt: number, canRoam: boolean) {
    if (!this.locked && !this.roaming && canRoam && time >= this.nextWalk) this.walk();
    if (!this.roaming && !this.working) return false;
    const target = this.route[0];
    if (target) {
      this.seat = Math.max(0, this.seat - dt * 2.5);
      if (this.seat === 0 && time >= this.waitUntil) this.move(target, dt, time);
    } else if (this.turn(seatedAngle, dt) < .015) {
      this.seat = Math.min(1, this.seat + dt * 2.25);
    }
    this.gait = THREE.MathUtils.damp(this.gait, this.speed / walkingSpeed, 18, dt);
    if (this.roaming && !this.route.length && this.seat === 1) {
      this.roaming = false;
      this.nextWalk = time + 7;
    }
    this.pose(time, this.gait);
    return true;
  }

  private walk() {
    const lane = new THREE.Vector2(aisleX, this.home.y);
    this.route = [lane, new THREE.Vector2(aisleX, lane.y + (this.desk.z > 0 ? -1.48 : 1.48)), lane, this.home];
    this.stride = this.waitUntil = 0;
    this.roaming = true;
  }

  private routeHome() {
    if (!this.roaming) return;
    const {position} = this.character.actor;
    this.waitUntil = 0;
    this.route = Math.abs(position.z - this.home.y) < .01 ? [this.home] :
      [new THREE.Vector2(aisleX, this.home.y), this.home];
  }

  private move(target: THREE.Vector2, dt: number, time: number) {
    const {position} = this.character.actor;
    const x = target.x - position.x, z = target.y - position.z;
    const distance = Math.hypot(x, z);
    if (distance > .005 && this.turn(Math.atan2(-x, -z), dt) > .10) {
      this.speed = 0;
      return;
    }
    this.speed = Math.min(this.speed + dt * acceleration, walkingSpeed, Math.sqrt(2 * acceleration * distance));
    const step = Math.min(distance, dt * this.speed);
    if (distance > .005 && distance > step) {
      position.x += x * step / distance; position.z += z * step / distance;
      this.stride += step * 7.6;
      return;
    }
    position.x = target.x; position.z = target.y;
    this.speed = 0;
    this.route.shift();
    if (this.route.length === 2 && !this.locked) this.waitUntil = time + .38;
  }

  private turn(angle: number, dt: number) {
    const rotation = this.character.actor.rotation;
    const delta = angleDelta(rotation.y, angle);
    const step = THREE.MathUtils.clamp(delta, -dt * turnSpeed, dt * turnSpeed);
    rotation.y += step;
    return Math.abs(delta - step);
  }

  private pose(time: number, gait: number) {
    const seated = THREE.MathUtils.smoothstep(this.seat, 0, 1);
    this.character.actor.position.y = floor - seated * .155 + gait * (1 - Math.cos(this.stride * 2)) * .012;
    this.character.rig.forEach(({thigh, shin, arm}, index) => {
      const cycle = index ? -1 : 1;
      const swing = Math.sin(this.stride) * cycle * gait;
      const lift = Math.max(0, Math.cos(this.stride - .55) * cycle) * gait;
      if (thigh) thigh.rotation.x = seated * 1.26 + swing * .37;
      if (shin) shin.rotation.x = -seated * 1.26 - lift * .41;
      if (arm) arm.rotation.x = -swing * .24 + seated * (.43 + (this.working ? Math.sin(time * 11 + index * 2) * .11 : 0));
    });
  }
}
