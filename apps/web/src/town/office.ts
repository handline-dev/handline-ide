// An office's drawing (Inside.tsx shows it): the room without its front wall, which the people are drawn into, and the front wall with
// its door as a picture of its own laid over them, see-through while a person touches it. Drawn again only when what it shows changes.
import { useMemo, useState } from 'react';
import { WebGLRenderer } from 'three';
import type { Ground } from '#src/town/founder.tsx';
import { groundOf } from '#src/town/ground.ts';
import type { House } from '#src/town/model.ts';
import { boxOf, castOf, type Box, type Cast } from '#src/town/people.ts';
import { DOOR, inFront, roomOf, type Room } from '#src/town/room.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade, type Picture } from '#src/town/shade.ts';
import { leavesOf, type Leaves } from '#src/town/door.tsx';
import type { Stage } from '#src/town/crowd.ts';
import { doorOf, seatsOf } from '#src/town/moves.ts';

let renderer: WebGLRenderer | null = null;

/** The front wall's picture: its image, its pixels (the door swings out of them), the door's box, and for each pixel column the wall's
 *  topmost row (the picture's height where the wall is not); `people` when a seated person reaches into it. */
export type Front = { url: string; pic: Picture; door: Box | null; top: Int32Array; people: boolean };
/** `stage`: who sits where, for the people's walks (crowd.ts). */
export type Drawn = { url: string; w: number; h: number; cast: Cast; ground: Ground; front: Front; leaves: Leaves | null; stage: Stage };

/** What a drawing shows: the bench width and each person's name and state, so a poll that changes nothing draws nothing. */
const sigOf = (house: House, cols: number) =>
  [house.key, cols, house.decisions.length, ...house.people.map((p) => `${p.key}:${p.name}:${p.activity}:${p.queue}:${p.cached}`)].join(
    ' ',
  );
/** The last drawings, by house, so an office is there at once when it comes back; the oldest goes past MOST. */
const drawings = new Map<string, { sig: string; drawn: Drawn }>();
const MOST = 4;

/** The room drawn with its people, as image URLs and where each person and desk is; laid out and drawn again only when what it shows
 *  changes, not on each poll. `cols` is null until the pane has a size. */
export function useDrawn(house: House, cols: number | null): Drawn | null {
  const sig = cols === null ? null : sigOf(house, cols);
  const lay = () => ({ sig, house: house.key, room: cols === null ? null : roomOf(house, cols) });
  const [laid, setLaid] = useState(lay);
  if (laid.sig !== sig) setLaid(lay());
  return useMemo(() => drawOffice(laid), [laid]);
}

const urlOf = (pic: Picture) => {
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [pic.w, pic.h];
  canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
  return canvas.toDataURL();
};

/** For each pixel column, the topmost row a picture covers; its height where it covers none. */
export function topsOf(pic: Picture): Int32Array {
  const top = new Int32Array(pic.w).fill(pic.h);
  for (let i = 0; i < pic.id.length; i++) if (pic.id[i] && Math.floor(i / pic.w) < top[i % pic.w]) top[i % pic.w] = Math.floor(i / pic.w);
  return top;
}

/** Whether a box on the picture reaches into the front wall: below the wall's top in any of its columns. */
export function touches(top: Int32Array, box: Box): boolean {
  const bottom = box.y + box.h;
  for (let x = Math.max(0, Math.floor(box.x)); x < Math.min(top.length, Math.ceil(box.x + box.w)); x++) if (top[x] < bottom) return true;
  return false;
}

/** The office for a layout: the kept drawing when it shows the same, else drawn now and kept. */
function drawOffice({ room, sig, house: key }: { room: Room | null; sig: string | null; house: string }): Drawn | null {
  const kept = drawings.get(key);
  if (!room || sig === null || kept?.sig === sig) return kept?.drawn ?? null;
  renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
  // Both drawn through the same camera, which the room's size alone sets.
  const frame = drawRoom(renderer, { ...room, blocks: room.blocks.filter((b) => !inFront(b)) });
  const pic = shade(frame);
  const wallFrame = drawRoom(renderer, { ...room, blocks: room.blocks.filter(inFront) });
  const wall = shade(wallFrame);
  const cast = castOf(pic, frame, room);
  const top = topsOf(wall);
  // The founder walks round the front wall too, as the people do.
  const ground = groundOf(pic, { ...frame, solid: [...frame.solid, ...wallFrame.solid] }, room, { speed: 1.4, stride: 0.16 });
  const drawn = {
    url: urlOf(pic),
    w: pic.w,
    h: pic.h,
    cast,
    ground,
    front: { url: urlOf(wall), pic: wall, door: boxOf(wall, wallFrame, DOOR), top, people: cast.figures.some((f) => touches(top, f.box)) },
    // Hinged by the corner, it opens into the room, away from the viewer.
    leaves: leavesOf(room.blocks, DOOR, 'right'),
    stage: { seats: seatsOf(room, ground.floor), door: doorOf(room), floor: ground.floor, people: room.people, layout: room.w },
  };
  drawings.delete(key);
  drawings.set(key, { sig, drawn });
  for (const old of drawings.keys()) if (drawings.size > MOST) drawings.delete(old);
  return drawn;
}
