// The office room drawn by three.js into three small buffers that shade.ts turns into pixel art: each surface's flat colour, how much
// sunlight reaches it (with shadows), and per pixel which object it shows, how far away and which way it faces. Floor boards, the view out
// of the window and the lines on a lit screen are drawn from world positions, so they line up whatever the room's size.
import * as THREE from 'three';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';
import type { Kind, Room } from '#src/town/room.ts';
import { PROJECT_GLSL, viewOf, type View } from '#src/town/view.ts';
import type { Rect } from '#src/town/walk.ts';
import { colourOf, KIND_COLOUR, LIT_KINDS, WALL_TOP } from '#src/town/palette.ts';

type RGB = [number, number, number];

const MODELS = `${import.meta.env.BASE_URL}town/models/`;
const SUN = new THREE.Vector3(0.45, -1, -0.3).normalize(); // from the windows, back and upper left

/** What a pixel's object is: a block's kind or a model's name, and the tag the room gave it. */
export type Meta = { kind: Kind | string; tag?: string };
/** `solid`: what stands on the floor, as footprints (the founder walks around them). */
export type Frame = View & { base: Uint8Array; light: Uint8Array; info: Float32Array; meta: Meta[]; solid: Rect[] };

/** Floor things the founder walks over. */
const WALK_OVER = new Set(['rugRectangle']);

const loader = new GLTFLoader();
const models = new Map<string, Promise<THREE.Mesh[]>>();

/** A model's meshes with one normal per face (flat facets, as pixel art wants them), loaded once. */
function model(name: string): Promise<THREE.Mesh[]> {
  let m = models.get(name);
  if (!m) {
    m = loader.loadAsync(`${MODELS}${name}.glb`).then((gltf) => {
      gltf.scene.updateMatrixWorld(true);
      const out: THREE.Mesh[] = [];
      gltf.scene.traverse((o) => {
        if (!(o instanceof THREE.Mesh)) return;
        const g = (o.geometry as THREE.BufferGeometry).toNonIndexed();
        g.applyMatrix4(o.matrixWorld);
        g.computeVertexNormals();
        const mesh = new THREE.Mesh(g, o.material);
        out.push(mesh);
      });
      return out;
    });
    models.set(name, m);
  }
  return m;
}

const VERTEX = /* glsl */ `
  uniform float uLift;
  varying vec3 vWorld;
  varying vec3 vNormal;
  varying float vDepth;
  void main() {
    vec3 transformed = position;
    ${PROJECT_GLSL}
    vWorld = tWorld.xyz;
    vNormal = normalize(mat3(modelMatrix) * normal);
    vDepth = -tTrue.z;
  }`;

/** Flat colour (uTop on a face turned up, where set), or floor boards, the window's view or a screen's lines (uPattern 1, 2, 3); alpha
 *  marks a surface lit from within. */
const BASE_FRAGMENT = /* glsl */ `
  uniform vec3 uColour;
  uniform vec3 uTop;
  uniform float uGlow;
  uniform int uPattern;
  varying vec3 vWorld;
  varying vec3 vNormal;
  void main() {
    vec3 c = uTop.r >= 0.0 && vNormal.y > 0.5 ? uTop : uColour;
    vec3 w = vWorld;
    if (uPattern == 1) {
      float pz = w.z / 0.16;
      float row = floor(pz);
      float px = (w.x + mod(row * 0.37, 1.3)) / 1.3;
      bool seam = (pz - row) < 0.12 || fract(px) < 0.015;
      c *= (1.0 + 0.035 * sin(row * 12.9898)) * (seam ? 0.9 : 1.0);
    } else if (uPattern == 2) {
      float bh = 0.62 + 0.18 * abs(sin(floor(w.x / 0.22) * 7.3));
      bool building = w.y < bh && w.y > 0.47;
      bool lit = building && mod(floor(w.x / 0.05), 2.0) == 0.0 && mod(floor(w.y / 0.05), 2.0) == 0.0;
      if (building) c = vec3(150.0, 170.0, 200.0) / 255.0;
      if (lit) c = vec3(200.0, 220.0, 240.0) / 255.0;
      if (w.y < 0.56 + 0.04 * sin(w.x * 31.0)) c = vec3(100.0, 170.0, 90.0) / 255.0;
    } else if (uPattern == 3) {
      float r = floor(w.y / 0.03);
      if (mod(r, 2.0) == 0.0 && sin(w.x * 53.0 + r * 2.1) > -0.2) c *= 0.8;
    }
    gl_FragColor = vec4(c, uGlow);
  }`;

/** Object id, true depth, and the facing normal packed into one float, 8 bits a component. */
const INFO_FRAGMENT = /* glsl */ `
  precision highp float;
  uniform float uId;
  varying vec3 vNormal;
  varying float vDepth;
  void main() {
    vec3 n = normalize(vNormal) * (gl_FrontFacing ? 1.0 : -1.0);
    vec3 q = floor((n * 0.5 + 0.5) * 255.0 + 0.5);
    gl_FragColor = vec4(uId, vDepth, q.x * 65536.0 + q.y * 256.0 + q.z, 1.0);
  }`;

type Pass = 'base' | 'light' | 'info';

/** Draws the room once into the three buffers. */
export async function drawRoom(renderer: THREE.WebGLRenderer, room: Room): Promise<Frame> {
  const view = viewOf(room);
  const uLift = { value: view.lift };
  const scene = new THREE.Scene();
  const meta: Meta[] = [{ kind: 'none' }];
  const mats: [THREE.Mesh, Record<Pass, THREE.Material>][] = [];
  const light = new THREE.MeshLambertMaterial({ color: 0xffffff, side: THREE.DoubleSide });
  light.onBeforeCompile = (shader) => {
    shader.uniforms.uLift = uLift;
    shader.vertexShader = `uniform float uLift;\n${shader.vertexShader.replace('#include <project_vertex>', PROJECT_GLSL)}`;
  };
  const add = (mesh: THREE.Mesh, id: number, colour: RGB, glow: boolean, pattern = 0, top: RGB = [-255, 0, 0]) => {
    const shader = { vertexShader: VERTEX, side: THREE.DoubleSide };
    const base = new THREE.ShaderMaterial({
      ...shader,
      fragmentShader: BASE_FRAGMENT,
      uniforms: {
        uLift,
        uColour: { value: new THREE.Vector3(...colour.map((v) => v / 255)) },
        uTop: { value: new THREE.Vector3(...top.map((v) => v / 255)) },
        uGlow: { value: glow ? 1 : 0 },
        uPattern: { value: pattern },
      },
    });
    const info = new THREE.ShaderMaterial({ ...shader, fragmentShader: INFO_FRAGMENT, uniforms: { uLift, uId: { value: id } } });
    mesh.castShadow = mesh.receiveShadow = true;
    mats.push([mesh, { base, light, info }]);
    scene.add(mesh);
  };

  const boxes: THREE.BufferGeometry[] = [];
  const parts = new Map<string, number>();
  for (const b of room.blocks) {
    const id = (b.part && parts.get(b.part)) || meta.push({ kind: b.kind, tag: b.tag }) - 1;
    if (b.part) parts.set(b.part, id);
    const size = b.to.map((v, i) => v - b.from[i]) as [number, number, number];
    const box = new THREE.BoxGeometry(...size);
    boxes.push(box);
    const mesh = new THREE.Mesh(box);
    mesh.position.set(...(b.from.map((v, i) => v + size[i] / 2) as [number, number, number]));
    const pattern = b.kind === 'floor' ? 1 : b.kind === 'glass' ? 2 : 0;
    const wall = b.kind === 'wall' || b.kind === 'side';
    add(mesh, id, KIND_COLOUR[b.kind], LIT_KINDS.includes(b.kind), pattern, wall ? WALL_TOP : undefined);
  }
  const loaded = await Promise.all(room.things.map((t) => model(t.model)));
  const solid: Rect[] = [];
  room.things.forEach((t, i) => {
    const id = meta.push({ kind: t.model, tag: t.tag }) - 1;
    const foot = new THREE.Box3();
    for (const part of loaded[i]) {
      const mesh = new THREE.Mesh(part.geometry);
      mesh.position.set(t.x, t.y ?? 0, t.z);
      mesh.rotation.y = t.rot ?? 0;
      mesh.scale.setScalar(t.scale ?? 1);
      const name = (part.material as THREE.MeshStandardMaterial).name;
      const { colour, glow } = colourOf(t, name, (part.material as THREE.MeshStandardMaterial).color);
      add(mesh, id, colour, glow, glow && t.model === 'computerScreen' ? 3 : 0);
      mesh.updateMatrixWorld();
      foot.expandByObject(mesh);
    }
    if (!t.y && !WALK_OVER.has(t.model)) solid.push({ x0: foot.min.x, z0: foot.min.z, x1: foot.max.x, z1: foot.max.z });
  });

  const sun = new THREE.DirectionalLight(0xffffff, Math.PI); // Lambert divides by pi: the light buffer holds N·L
  const middle = new THREE.Vector3(room.w / 2, 0, room.d / 2);
  sun.position.copy(middle).addScaledVector(SUN, -20);
  sun.target.position.copy(middle);
  sun.castShadow = true;
  const r = Math.max(room.w, room.d);
  Object.assign(sun.shadow.camera, { left: -r, right: r, top: r, bottom: -r, near: 1, far: 50 });
  sun.shadow.mapSize.set(2048, 2048);
  sun.shadow.bias = -0.0008;
  scene.add(sun, sun.target);

  const run = (pass: Pass, type: THREE.TextureDataType) => {
    for (const [mesh, m] of mats) mesh.material = m[pass];
    const target = new THREE.WebGLRenderTarget(view.w, view.h, { type, minFilter: THREE.NearestFilter, magFilter: THREE.NearestFilter });
    renderer.setRenderTarget(target);
    renderer.setClearColor(0x000000, 0);
    renderer.clear();
    renderer.render(scene, view.camera);
    const out = type === THREE.FloatType ? new Float32Array(view.w * view.h * 4) : new Uint8Array(view.w * view.h * 4);
    renderer.readRenderTargetPixels(target, 0, 0, view.w, view.h, out);
    renderer.setRenderTarget(null);
    target.dispose();
    return out;
  };
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = THREE.BasicShadowMap;
  const frame: Frame = {
    ...view,
    base: run('base', THREE.UnsignedByteType) as Uint8Array,
    light: run('light', THREE.UnsignedByteType) as Uint8Array,
    info: run('info', THREE.FloatType) as Float32Array,
    meta,
    solid,
  };
  for (const [, m] of mats) {
    m.base.dispose();
    m.info.dispose();
  }
  for (const g of boxes) g.dispose(); // the models' geometry stays cached for the next room
  light.dispose();
  return frame;
}
