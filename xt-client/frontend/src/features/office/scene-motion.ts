import * as THREE from 'three';
import {mustBeSeated, type SceneMember} from './scene-contract';
import type {instantiateCharacter} from './scene-models';
import {aisleX, seatOffset, type Workstation} from './scene-workstations';

type PosedCharacter = ReturnType<typeof instantiateCharacter>;
const seatedAngle = Math.PI;
const floor = .035;
const acceleration = 5;
const turnSpeed = 7;

function angleDelta(from: number, to: number) {
  return Math.atan2(Math.sin(to - from), Math.cos(to - from));
}

type WalkStop = {position: THREE.Vector2; pause?: {duration: number; look: number}};

export class OfficeMotion {
  private seat = 1;
  private route: WalkStop[] = [];
  private nextWalk: number;
  private locked: boolean;
  private working = false;
  private stride = 0;
  private speed = 0;
  private pace = 1.5;
  private gait = 0;
  private waitUntil = 0;
  private lookAngle = 0;
  private readonly home: THREE.Vector2;
  roaming = false;

  constructor(private character: PosedCharacter, member: SceneMember, desk: Workstation, time: number, private random = Math.random) {
    this.home = new THREE.Vector2(desk.x, desk.z - seatOffset);
    this.nextWalk = time + this.sample(2.5, 8);
    this.locked = mustBeSeated(member);
    this.working = member.status === 'running';
    this.snapHome();
  }

  update(member: SceneMember, time: number) {
    const locked = mustBeSeated(member);
    if (locked && !this.locked) this.routeHome();
    if (!locked && this.locked) this.nextWalk = time + this.sample(4, 11);
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
      if (this.seat === 0) {
        if (time < this.waitUntil) this.turn(this.lookAngle, dt, 3);
        else this.move(target, dt, time);
      }
    } else if (this.turn(seatedAngle, dt) < .015) {
      this.seat = Math.min(1, this.seat + dt * 2.25);
    }
    this.gait = THREE.MathUtils.damp(this.gait, this.speed / this.pace, 13, dt);
    if (this.roaming && !this.route.length && this.seat === 1) {
      this.roaming = false;
      this.nextWalk = time + this.sample(6, 17);
    }
    this.pose(time, this.gait);
    return true;
  }

  private walk() {
    const x = aisleX + this.sample(-.13, .13);
    const z = this.home.y;
    const lane = {position: new THREE.Vector2(x, z)};
    // The floor always reaches past this chair, even if desks farther down are removed.
    const low = Math.min(-4.32, z - 1);
    const high = 3.65;
    const direction = this.random() < .5 ? -1 : 1;
    const distance = this.sample(.95, 3.15);
    const destination = THREE.MathUtils.clamp(z + direction * distance, low, high);
    this.pace = this.sample(1.1, 1.65);
    this.route = [lane, {
      position: new THREE.Vector2(x, destination),
      pause: this.random() < .72 ? {duration: this.sample(.6, 1.8), look: this.sample(-1.1, 1.1)} : undefined,
    }, lane, {position: this.home}];
    this.stride = this.waitUntil = 0;
    this.roaming = true;
  }

  private routeHome() {
    if (!this.roaming) return;
    const {position} = this.character.actor;
    this.waitUntil = 0;
    const home = {position: this.home};
    this.route = Math.abs(position.z - this.home.y) < .01 ? [home] :
      [{position: new THREE.Vector2(position.x, this.home.y)}, home];
  }

  private move({position: target, pause}: WalkStop, dt: number, time: number) {
    const {position, rotation} = this.character.actor;
    const x = target.x - position.x, z = target.y - position.z;
    const distance = Math.hypot(x, z);
    if (distance > .005 && this.turn(Math.atan2(-x, -z), dt) > .10) {
      this.speed = 0;
      return;
    }
    this.speed = Math.min(this.speed + dt * acceleration, this.pace, Math.sqrt(2 * acceleration * distance));
    const step = Math.min(distance, dt * this.speed);
    if (distance > .005 && distance > step) {
      position.x += x * step / distance; position.z += z * step / distance;
      this.stride += step * 7.6;
      return;
    }
    position.x = target.x; position.z = target.y;
    this.speed = 0;
    this.route.shift();
    if (pause) {
      this.waitUntil = time + pause.duration;
      this.lookAngle = rotation.y + pause.look;
    }
  }

  private sample(min: number, max: number) {return min + (max - min) * this.random();}

  private turn(angle: number, dt: number, rate = turnSpeed) {
    const rotation = this.character.actor.rotation;
    const delta = angleDelta(rotation.y, angle);
    const step = THREE.MathUtils.clamp(delta, -dt * rate, dt * rate);
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
