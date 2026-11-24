// The office camera: tilted down by PITCH, yet every vertical edge stays a vertical pixel column. A point is drawn where the tilted camera
// sees its spot on the floor, then lifted straight up the screen by its height, foreshortened as the camera would and stretched by LIFT so
// furniture keeps its stature. The shaders (scene3d.ts) and `toPixel` here do the same sum.
import { Matrix4, PerspectiveCamera, Vector3 } from 'three';
import { SLAB, WALL_T, type Room } from '#src/town/room.ts';

const PITCH = (57 * Math.PI) / 180;
const LIFT = 1.5;
const DISTANCE = 24; // metres from the room's middle: far enough that the perspective stays gentle
/** How tall a standing person is in pixels, so the drawn people (about this tall in the sheet) need no resizing. */
const PERSON_PX = 39;
const PERSON_M = 0.86;
const MARGIN = 3;

export type View = {
  camera: PerspectiveCamera;
  /** Camera-space up per world metre of height, the shaders' lift. */
  lift: number;
  w: number;
  h: number;
  toPixel: (x: number, y: number, z: number) => [number, number];
};

/** The camera on a room and the picture's size: whole pixels, the room and its walls inside with a small margin. */
export function viewOf(room: Room): View {
  const centre = new Vector3(room.w / 2, 0, room.d / 2);
  const eye = centre.clone().add(new Vector3(0, Math.sin(PITCH), Math.cos(PITCH)).multiplyScalar(DISTANCE));
  const camera = new PerspectiveCamera();
  camera.position.copy(eye);
  camera.lookAt(centre);
  camera.updateMatrixWorld();
  const view = camera.matrixWorldInverse;
  const up = new Vector3(0, 1, 0).transformDirection(camera.matrixWorld).y; // the camera's up axis, in world y
  const lift = up * LIFT;
  /** Where a point lands before scaling: x and y over the depth of its floor spot. */
  const raw = (x: number, y: number, z: number): [number, number] => {
    const g = new Vector3(x, 0, z).applyMatrix4(view);
    return [g.x / -g.z, (g.y + y * lift) / -g.z];
  };
  const [, foot] = raw(centre.x, 0, centre.z);
  const [, head] = raw(centre.x, PERSON_M, centre.z);
  const s = PERSON_PX / (head - foot);
  const [t, H] = [WALL_T, room.wallH];
  const corners = [-t, room.w + t].flatMap((x) =>
    [
      [H, -t],
      [H, room.d],
      [-SLAB, room.d],
    ].map(([y, z]) => raw(x, y, z)),
  );
  const xs = corners.map((c) => c[0]);
  const ys = corners.map((c) => c[1]);
  const w = Math.ceil((Math.max(...xs) - Math.min(...xs)) * s) + 2 * MARGIN;
  const h = Math.ceil((Math.max(...ys) - Math.min(...ys)) * s) + 2 * MARGIN;
  const ox = MARGIN - Math.min(...xs) * s;
  const oy = MARGIN + Math.max(...ys) * s;
  // The projection that puts raw (x, y) at pixel (s x + ox, oy - s y); the shaders hand it the floor spot's x and y scaled to the
  // point's own depth, so the depth test still sees the true depth.
  const [n, f] = [1, 100];
  // prettier-ignore
  camera.projectionMatrix = new Matrix4().set(
    (2 * s) / w, 0, 1 - (2 * ox) / w, 0,
    0, (2 * s) / h, (2 * oy) / h - 1, 0,
    0, 0, -(f + n) / (f - n), (-2 * f * n) / (f - n),
    0, 0, -1, 0,
  );
  camera.projectionMatrixInverse.copy(camera.projectionMatrix).invert();
  const toPixel = (x: number, y: number, z: number): [number, number] => {
    const [rx, ry] = raw(x, y, z);
    return [s * rx + ox, oy - s * ry];
  };
  return { camera, lift, w, h, toPixel };
}

/** The vertex shader's ending that does the same sum as `toPixel`; it needs `transformed` and a `uLift` uniform. */
export const PROJECT_GLSL = /* glsl */ `
  vec4 tWorld = modelMatrix * vec4(transformed, 1.0);
  vec4 tFloor = viewMatrix * vec4(tWorld.x, 0.0, tWorld.z, 1.0);
  vec4 tTrue = viewMatrix * tWorld;
  float tK = tTrue.z / tFloor.z;
  vec4 mvPosition = vec4(tFloor.x * tK, (tFloor.y + tWorld.y * uLift) * tK, tTrue.z, 1.0);
  gl_Position = projectionMatrix * mvPosition;
`;
