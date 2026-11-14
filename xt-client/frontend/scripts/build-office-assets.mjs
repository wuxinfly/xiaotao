import {mkdir, writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import * as THREE from 'three';
import {RoundedBoxGeometry} from 'three/addons/geometries/RoundedBoxGeometry.js';
import {mergeGeometries, mergeVertices} from 'three/addons/utils/BufferGeometryUtils.js';

const palette = {
  plinth: '#264454', floor: '#c8d8db', tile: '#b4cbd1', wall: '#55858d', trim: '#254e5c',
  wood: '#dbb68a', shell: '#e9f0ed', sofa: '#34837f', cushion: '#65aca0', rug: '#487b8a',
  ink: '#142b3d', ocean: '#237e9f', deep: '#195774', glow: '#94e9dd', coral: '#f0a780',
  leaf: '#428e76', stem: '#285d54', primary: '#56baa9', secondary: '#305176', face: '#bcfaee',
  window: '#55b6c7', horizon: '#26778e', sunDisc: '#fff0bd', nightDisc: '#c7e9ff', nightStars: '#b8e6ff', lamp: '#ffe1ac',
};
const surfaces = {ocean: '#124457', deep: '#103146', glow: '#437d73', face: '#578d81', sunDisc: '#ffca78', nightDisc: '#94cafa', nightStars: '#639eee', lamp: '#d09c51'};
const materialNames = Object.keys(palette);
const paint = name => {
  const color = new THREE.Color(palette[name]);
  return {name, pbrMetallicRoughness: {baseColorFactor: [...color.toArray(), 1], roughness: .72, metallicFactor: 0}, ...(surfaces[name] && {emissiveFactor: new THREE.Color(surfaces[name]).toArray()})};
};

class Model {
  parts = new Map();
  add(color, geometry, at, rotation = [0, 0, 0]) {
    geometry.applyMatrix4(new THREE.Matrix4().makeRotationFromEuler(new THREE.Euler(...rotation)).setPosition(...at));
    geometry.deleteAttribute('uv');
    const batch = this.parts.get(color) || [];
    batch.push(geometry.index ? geometry.toNonIndexed() : geometry);
    this.parts.set(color, batch);
  }
  box(color, at, size, radius = .07, rotation) {
    const r = Math.min(radius, ...size.map(n => n * .48));
    this.add(color, new RoundedBoxGeometry(...size, 1, r), at, rotation);
  }
  oval(color, at, size, rotation) {
    this.add(color, new THREE.SphereGeometry(1, 14, 10).scale(...size), at, rotation);
  }
  tube(color, at, bottom, top, height) {
    this.add(color, new THREE.CylinderGeometry(top, bottom, height, 16), at);
  }
}

function buildWalls(m) {
  m.box('plinth', [0, -.25, 0], [14.2, .50, 10.6], .20);
  m.box('floor', [0, -.05, 0], [13.98, .17, 10.4], .07);
  m.box('wood', [0, -.135, -5.25], [12.95, .045, .08], .02);
  for (const x of [-4.6, -2.3, 0, 2.3, 4.6]) m.box('tile', [x, .037, -.3], [.018, .009, 9.6], .003);
  m.box('wall', [0, 1.91, 5.09], [14, 3.9, .27], .10);
  m.box('wall', [-6.87, 1.21, 1.44], [.27, 2.5, 7.55], .10);
  m.box('trim', [0, .24, 4.94], [13.78, .39, .11]);
  m.box('trim', [-6.71, .24, 1.45], [.10, .39, 7.37]);
  m.box('trim', [-6.86, 2.47, 1.42], [.36, .14, 7.60]);
  m.box('trim', [-1.30, 2.56, 4.91], [8.95, 2.07, .22], .2);
  for (const x of [-4.19, -1.30, 1.59]) {
    m.box('window', [x, 2.56, 4.77], [2.73, 1.78, .12], .18);
    m.box('horizon', [x, 2.10, 4.70], [2.57, .67, .06], .15);
    m.box('glow', [x, 2.40, 4.66], [2.37, .020, .020], .009);
  }
  m.oval('sunDisc', [-.67, 2.91, 4.68], [.27, .27, .026]);
  m.oval('nightDisc', [-.67, 2.91, 4.68], [.21, .21, .026]);
  for (const [x, y, r] of [[-4.9, 3.08, .027], [-4.37, 2.86, .017], [-3.6, 2.61, .023], [-2.28, 2.93, .021], [-1.56, 2.67, .023], [.9, 2.78, .022], [1.47, 3.18, .027], [2.33, 2.93, .019]]) {
    m.oval('nightStars', [x, y, 4.67], [r, r, .015]);
  }
  m.box('glow', [-6.69, 1.87, 1.52], [.06, .055, 6.25]);
  m.box('trim', [-6.66, 1.27, .92], [.17, .86, 1.28], .13);
  for (const z of [.58, .92, 1.26]) m.oval('glow', [-6.55, 1.25, z], [.03, .058, .058]);
}

function desk(coordinator = false) {
  const m = new Model(), x = 0, z = 0;
  m.box(coordinator ? 'sofa' : 'wood', [x, .96, z], [coordinator ? 3.33 : 2.40, .16, 1.12], .10);
  for (const dx of [-.97, .97]) m.box('trim', [x + dx, .49, z], [.13, .89, .88]);
  m.box('ink', [x, 1.42, z + .14], [1.18, .62, .14], .10);
  m.box('ocean', [x, 1.42, z + .052], [1.02, .46, .044], .055);
  for (let i = 0; i < 3; i++) m.box('glow', [x - .18 + (i % 2) * .06, 1.53 - i * .09, z + .024], [.47 - i * .085, .025, .021], .007);
  m.box('ink', [x, 1.07, z + .14], [.36, .15, .26]);
  m.box('shell', [x, 1.06, z - .29], [.69, .045, .24], .026);
  m.tube('coral', [x + .93, 1.15, z - .12], .09, .10, .22);
  m.box('cushion', [x - .87, 1.06, z + .03], [.38, .06, .32], .03);
  m.tube('trim', [-.9, 1.22, .32], .035, .035, .34);
  m.box('lamp', [-.9, 1.43, .30], [.34, .12, .27], .055);
  if (coordinator) {
    m.box('rug', [0, .050, -.36], [3.68, .042, 2.23], .02);
    m.box('glow', [0, .89, -.574], [1.58, .028, .029], .012);
  }
  return m;
}

function chair() {
  const m = new Model();
  m.tube('ink', [0, .09, 0], .31, .27, .10);
  m.tube('trim', [0, .26, 0], .066, .062, .29);
  m.box('sofa', [0, .42, 0], [.79, .14, .68], .066);
  m.box('trim', [0, .61, -.29], [.1, .30, .095], .03);
  m.box('cushion', [0, .80, -.31], [.69, .27, .14], .065);
  return m;
}

function floorExtension() {
  const m = new Model();
  m.box('plinth', [0, -.25, 0], [14.2, .50, 3.30], .13);
  m.box('floor', [0, -.05, 0], [13.98, .17, 3.30], .05);
  for (const x of [-.89, 6.24]) m.box('tile', [x, .041, 0], [.048, .014, 3.25], .006);
  return m;
}

function plant(m, x, z, scale = 1) {
  m.tube('shell', [x, .27 * scale, z], .23 * scale, .33 * scale, .47 * scale);
  m.tube('trim', [x, .51 * scale, z], .32 * scale, .32 * scale, .08 * scale);
  for (let i = 0; i < 7; i++) {
    const a = i * 2.4;
    m.oval(i % 2 ? 'leaf' : 'cushion',
      [x + Math.sin(a) * .27 * scale, (.77 + (i % 3) * .15) * scale, z + Math.cos(a) * .27 * scale],
      [.14 * scale, .39 * scale, .08 * scale], [.58, a, .18]);
  }
}

function lounge(m) {
  m.box('rug', [-4.77, .055, -1.68], [3.91, .045, 4.16], .022);
  m.box('sofa', [-5.88, .39, -1.4], [1.14, .42, 2.54], .17);
  m.box('sofa', [-6.33, .74, -1.4], [.32, .77, 2.54], .15);
  for (const z of [-2.54, -.26]) m.box('sofa', [-5.85, .64, z], [1.12, .47, .25], .11);
  for (const z of [-1.92, -.88]) m.box('cushion', [-5.81, .65, z], [.72, .18, .93], .085);
  m.box('coral', [-6.08, .89, -.85], [.17, .40, .46], .075, [.16, 0, .22]);
  m.tube('trim', [-4.47, .23, -1.61], .27, .16, .42);
  m.tube('wood', [-4.47, .45, -1.61], .59, .59, .10);
  m.box('shell', [-4.58, .53, -1.54], [.33, .06, .26], .03, [0, .25, 0]);
  m.tube('coral', [-4.22, .57, -1.79], .073, .077, .15);
}

function room() {
  const m = new Model();
  buildWalls(m);
  lounge(m);
  for (const x of [-.89, 6.24]) m.box('tile', [x, .041, -2.45], [.048, .014, 5.58], .006);
  plant(m, -5.68, 3.65, 1.25); plant(m, 6.18, 4.16, 1.17); plant(m, -6.11, -3.88, .92);
  return m;
}

function archive() {
  const m = new Model();
  const x = 5.06, z = 4.37;
  m.box('trim', [x, 1.25, z], [1.65, 2.50, .73], .13);
  m.box('ocean', [x, 1.4, z - .35], [1.34, 1.8, .1], .045);
  for (const height of [.64, 1.31, 1.98]) {
    m.box('wood', [x, height, z - .24], [1.42, .09, .47]);
    for (const [i, color] of ['shell', 'coral', 'cushion', 'glow'].entries()) {
      m.box(color, [x - .48 + i * .26, height + .23, z - .28], [.19, .37 + (i % 2) * .08, .28], .03);
    }
  }
  m.box('glow', [x, 2.34, z - .38], [.65, .043, .032], .015);
  return m;
}

function limb(side, part) {
  const m = new Model();
  if (part === 'arm') {
    m.oval('primary', [side * .04, -.15, 0], [.137, .24, .151], [0, 0, side * .15]);
    m.oval('secondary', [side * .074, -.33, -.04], [.126, .123, .141]);
  } else if (part === 'thigh') {
    m.oval('secondary', [0, -.12, 0], [.143, .19, .144]);
  } else {
    m.oval('secondary', [0, -.095, 0], [.129, .15, .13]);
    m.box('shell', [0, -.245, -.085], [.32, .17, .43], .08);
  }
  return m;
}

function character(shape) {
  const m = new Model();
  m.box('primary', [0, .75, 0], [.82, .72, .58], .25);
  m.box('secondary', [0, .86, -.304], [.36, .19, .04], .019);
  m.oval('face', [-.10, .86, -.332], [.037, .035, .019]);
  m.box('shell', [.077, .858, -.333], [.116, .025, .021], .01);
  if (shape === 'square') m.box('primary', [0, 1.45, 0], [1.08, .79, .84], .23);
  else if (shape === 'hexagon') m.add('primary', new THREE.CylinderGeometry(.57, .57, .73, 6).scale(1, 1, .76), [0, 1.45, 0], [0, Math.PI / 6, 0]);
  else if (shape === 'triangle') {
    const cap = new THREE.Shape();
    cap.moveTo(-.52, -.29); cap.quadraticCurveTo(-.57, -.36, -.41, -.36);
    cap.lineTo(.41, -.36); cap.quadraticCurveTo(.57, -.36, .52, -.29);
    cap.lineTo(.12, .43); cap.quadraticCurveTo(0, .62, -.12, .43); cap.lineTo(-.52, -.29);
    m.add('primary', new THREE.ExtrudeGeometry(cap, {depth: .51, curveSegments: 4, bevelEnabled: true, bevelThickness: .13, bevelSize: .075, bevelSegments: 2}), [0, 1.50, -.255]);
  }
  else m.oval('primary', [0, 1.45, 0], [.55, .43, .44]);
  m.box('ink', [0, 1.45, -.35], [.83, .46, .29], .14);
  for (const side of [-1, 1]) {
    m.oval('face', [side * .155, 1.47, -.503], [.060, .088, .020]);
    m.oval('secondary', [side * .515, 1.44, .021], [.082, .155, .17]);
  }
  m.box('face', [0, 1.355, -.496], [.071, .025, .026], .011);
  if (shape === 'triangle') {
    m.add('secondary', new THREE.ConeGeometry(.21, .38, 3), [-.21, 1.89, 0], [0, .3, -.3]);
    m.oval('face', [-.27, 2.06, 0], [.057, .057, .057]);
  } else {
    m.oval('secondary', [-.32, 1.79, .015], [.059, .18, .059], [0, 0, .35]);
    m.oval('face', [-.38, 1.95, .015], [.066, .066, .066]);
  }
  return m;
}

const gltf = {
  asset: {version: '2.0', generator: 'XiaoTao procedural office'},
  scene: 0, scenes: [{nodes: []}],
  nodes: [], meshes: [], materials: materialNames.map(paint), accessors: [], bufferViews: [], buffers: [],
};
const chunks = [];
let byteLength = 0;
function accessor(attribute, type, boundsRequired = false) {
  const data = Buffer.from(attribute.array.buffer, attribute.array.byteOffset, attribute.array.byteLength);
  const index = type === 'SCALAR';
  const bufferView = gltf.bufferViews.push({buffer: 0, byteOffset: byteLength, byteLength: data.length, target: index ? 34963 : 34962}) - 1;
  chunks.push(data);
  byteLength += data.length;
  const item = {bufferView, componentType: index ? 5125 : 5126, count: attribute.count, type};
  if (boundsRequired) {
    const bounds = new THREE.Box3().setFromBufferAttribute(attribute);
    item.min = bounds.min.toArray(); item.max = bounds.max.toArray();
  }
  return gltf.accessors.push(item) - 1;
}
function addModel(name, model, translation, children) {
  const primitives = [...model.parts].map(([color, batch]) => {
    const geometry = mergeVertices(mergeGeometries(batch));
    return {
      attributes: {POSITION: accessor(geometry.attributes.position, 'VEC3', true), NORMAL: accessor(geometry.attributes.normal, 'VEC3')},
      indices: accessor({array: Uint32Array.from(geometry.index.array), count: geometry.index.count}, 'SCALAR'),
      material: materialNames.indexOf(color),
    };
  });
  return gltf.nodes.push({name, mesh: gltf.meshes.push({primitives}) - 1, translation, children}) - 1;
}

const roots = gltf.scenes[0].nodes;
roots.push(addModel('OfficeRoom', room()), addModel('OfficeArchive', archive()), addModel('OfficeDesk', desk()), addModel('CoordinatorDesk', desk(true)), addModel('OfficeChair', chair()), addModel('OfficeFloorExtension', floorExtension()));
for (const shape of ['round', 'square', 'hexagon', 'triangle']) {
  const rig = [];
  for (const side of [-1, 1]) {
    const label = side === -1 ? 'Left' : 'Right';
    const shin = addModel(`${label}Shin`, limb(side, 'shin'), [0, -.28, 0]);
    rig.push(addModel(`${label}Thigh`, limb(side, 'thigh'), [side * .216, .61, 0], [shin]));
    rig.push(addModel(`${label}Arm`, limb(side, 'arm'), [side * .43, .94, -.027]));
  }
  roots.push(addModel(shape === 'round' ? 'QCharacter' : `QCharacter-${shape}`, character(shape), undefined, rig));
}
gltf.buffers.push({byteLength});
function chunk(data, tag, padding) {
  const payload = Buffer.alloc(Math.ceil(data.length / 4) * 4, padding);
  data.copy(payload);
  const header = Buffer.alloc(8);
  header.writeUInt32LE(payload.length, 0);
  header.writeUInt32LE(tag, 4);
  return Buffer.concat([header, payload]);
}
const json = chunk(Buffer.from(JSON.stringify(gltf)), 0x4e4f534a, 0x20);
const bin = chunk(Buffer.concat(chunks), 0x004e4942, 0);
const header = Buffer.alloc(12);
header.writeUInt32LE(0x46546c67, 0);
header.writeUInt32LE(2, 4);
header.writeUInt32LE(12 + json.length + bin.length, 8);
const output = resolve('public/models');
await mkdir(output, {recursive: true});
await writeFile(resolve(output, 'office.glb'), Buffer.concat([header, json, bin]));
await writeFile(resolve(output, 'LICENSE.txt'), 'Original procedural geometry and artwork created for XiaoTao. No third-party meshes, textures, fonts, or external downloads. Distributed under the repository license.\n');
console.log(`Office assets: ${Math.round(bin.length / 1024)} KiB, ${gltf.meshes.reduce((count, mesh) => count + mesh.primitives.length, 0)} material batches`);
