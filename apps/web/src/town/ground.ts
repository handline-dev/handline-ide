// What the founder walks on in a drawn room, an office or the city: the room's footprints as a walk grid, and the camera's sums both ways.
import { Vector3 } from 'three';
import type { Ground } from '#src/town/founder.tsx';
import type { Room } from '#src/town/room.ts';
import type { Frame } from '#src/town/scene3d.ts';
import type { Picture } from '#src/town/shade.ts';
import { floorOf, type Grid, type Spot } from '#src/town/walk.ts';
import { SCALE } from '#src/town/people.ts';
import { PERSON_PX } from '#src/town/view.ts';

/** `speed` in metres a second and `stride` between the two feet frames; `grid` the walk grid's step and margins (an office's when left out). */
export function groundOf(pic: Picture, frame: Frame, room: Room, walk: { speed: number; stride: number; grid?: Grid }): Ground {
  const camera = frame.camera;
  const toCamera = camera.matrixWorldInverse;
  // On the floor the picture is the camera's own projection, so a pixel's ray meets the floor where that pixel shows it.
  const toFloor = (px: number, py: number): Spot => {
    const p = new Vector3((px / frame.w) * 2 - 1, 1 - (py / frame.h) * 2, 0.5).unproject(camera).sub(camera.position);
    const t = -camera.position.y / p.y;
    return [camera.position.x + t * p.x, camera.position.z + t * p.z];
  };
  return {
    pic,
    kinds: frame.meta.map((m) => m.kind),
    floor: floorOf(room.w, room.d, frame.solid, walk.grid),
    entry: room.entry,
    speed: walk.speed,
    stride: walk.stride,
    toPixel: frame.toPixel,
    toFloor,
    depthOf: (x, y, z) => -new Vector3(x, y, z).applyMatrix4(toCamera).z,
    sprite: (SCALE * (room.personPx ?? PERSON_PX)) / PERSON_PX,
  };
}
