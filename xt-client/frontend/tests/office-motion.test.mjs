import assert from 'node:assert/strict';
import {after, before, test} from 'node:test';
import {createServer} from 'vite';
import {Group} from 'three';

let vite, OfficeMotion, OfficeWorkstations;
const member = (id, role = 'project', status = 'idle') => ({id, role, status});
const mesh = () => new Group();

before(async () => {
  vite = await createServer({configFile: false, server: {middlewareMode: true}, appType: 'custom'});
  ({OfficeMotion} = await vite.ssrLoadModule('/src/features/office/scene-motion.ts'));
  ({OfficeWorkstations} = await vite.ssrLoadModule('/src/features/office/scene-workstations.ts'));
});
after(async () => vite?.close());

test('office expansion adds only occupied desks and preserves other seats when removing a member', () => {
  const desks = new OfficeWorkstations({desk: mesh(), coordinatorDesk: mesh(), chair: mesh(), floorExtension: mesh()});
  const coordinator = member('coordinator', 'coordinator');
  assert.equal(desks.update([coordinator]), 0);
  assert.equal(desks.furniture.children.length, 1);
  const fixed = desks.get(coordinator.id);
  const team = Array.from({length: 12}, (_, index) => member('employee-' + index));
  assert.ok(desks.update([...team, coordinator]) > 0);
  assert.equal(new Set(team.map(({id}) => {
    const {x, z} = desks.get(id);
    return `${x},${z}`;
  })).size, team.length);
  const last = desks.get(team.at(-1).id);
  desks.update([coordinator, ...team.slice(1)]);
  assert.equal(desks.get(team[0].id), undefined);
  assert.equal(desks.get(team.at(-1).id), last);
  assert.equal(desks.get(coordinator.id), fixed);
  assert.equal(desks.update([coordinator]), 0);
  assert.equal(desks.furniture.children.length, 1);
});

function createMotion(person) {
  const actor = mesh();
  const rig = [{thigh: mesh(), shin: mesh(), arm: mesh()}, {thigh: mesh(), shin: mesh(), arm: mesh()}];
  const motion = new OfficeMotion({actor, rig}, person, {x: .78, z: 2.45}, 0);
  const home = actor.position.clone();
  const tick = (start, end) => {for (let i = start; i < end; i++) motion.step(i * .025, .025, true);};
  return {actor, rig, home, motion, tick};
}

test('idle member walks away, gets back to the same desk and bends their knees to sit', () => {
  const {actor, rig, home, motion, tick} = createMotion(member('employee'));
  tick(240, 316);
  assert.ok(actor.position.distanceTo(home) > .4);
  assert.ok(Math.abs(rig[0].thigh.rotation.x) < .5);
  tick(316, 565);
  assert.equal(motion.roaming, false);
  assert.ok(actor.position.distanceTo(home) < .001);
  assert.ok(rig[0].thigh.rotation.x > 1 && rig[0].shin.rotation.x < -1);
});

test('the character stands before moving and faces the direction of each step', () => {
  const {actor, home, motion, rig} = createMotion(member('employee'));
  let moved = false, seatedAgain = false;
  for (let i = 0; i < 570; i++) {
    const previous = actor.position.clone();
    motion.step(i * .025, .025, true);
    const dx = actor.position.x - previous.x, dz = actor.position.z - previous.z;
    if (Math.hypot(dx, dz) > .006) {
      moved = true;
      assert.ok(rig[0].thigh.rotation.x < .5, 'do not translate while seated');
      const alignment = (-Math.sin(actor.rotation.y) * dx - Math.cos(actor.rotation.y) * dz) / Math.hypot(dx, dz);
      assert.ok(alignment > .99, 'no sideways sliding when the route turns');
    }
    if (moved && !motion.roaming) {
      assert.ok(actor.position.distanceTo(home) < .001);
      seatedAgain = true;
      break;
    }
  }
  assert.ok(moved && seatedAgain);
});

test('work interrupts a walk, and coordinator and queued executor stay at their desks', () => {
  const idle = member('employee');
  const {actor, home, motion, tick} = createMotion(idle);
  tick(240, 320);
  assert.ok(actor.position.distanceTo(home) > 1);
  motion.update({...idle, status: 'running'}, 8);
  tick(320, 660);
  assert.equal(motion.roaming, false);
  assert.ok(actor.position.distanceTo(home) < .001);
  for (const person of [member('coordinator', 'coordinator'), {...member('queued', 'executor'), taskId: 'task-id', taskStatus: 'queued'}]) {
    const seated = createMotion(person);
    seated.tick(0, 960);
    assert.ok(seated.actor.position.equals(seated.home));
  }
});
