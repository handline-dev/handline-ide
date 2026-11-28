// An office's drawing (Inside.tsx shows it): the room without its front wall, which the people are drawn into, and the front wall with
// its door as a picture of its own laid over them, see-through while a person touches it. Drawn again only when what it shows changes.
import { useMemo, useState } from 'react';
import { WebGLRenderer } from 'three';
import type { Ground } from '#src/town/founder.tsx';
import { groundOf } from '#src/town/ground.ts';
import type { House } from '#src/town/model.ts';
import { boxOf, castOf, type Box, type Cast } from '#src/town/people.ts';
import { DOOR, inFront, type Room } from '#src/town/room.ts';
import { fittedRoom } from '#src/town/fit.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade, type Picture } from '#src/town/shade.ts';
import { leavesOf, type Leaves } from '#src/town/door.tsx';
import type { Stage } from '#src/town/crowd.ts';
import { doorOf, seatsOf } from '#src/town/moves.ts';
import { screensOf, type Screen } from '#src/town/monitors.tsx';

let renderer: WebGLRenderer | null = null;

/** The front wall's picture: its image, its pixels (the door swings out of them), the door's box, and for each pixel column the wall's
 *  topmost row (the picture's height where the wall is not); `people` when a seated person reaches into it. */
export type Front = { url: string; pic: Picture; door: Box | null; top: Int32Array; people: boolean };
/** `stage`: who sits where, for the people's walks (crowd.ts); `screens`: the working agents' screens, which move. */
export type Drawn = {
  url: string;
  w: number;
  h: number;
  cast: Cast;
  ground: Ground;
  front: Front;
  leaves: Leaves | null;
  stage: Stage;
  screens: Screen[];
};

/** What a drawing shows: its decisions, each person's name and state, and whose screen stays dark, so a poll that changes nothing draws
 *  nothing. */
const sigOf = (house: House, dark: string) =>
  [house.key, house.decisions.length, ...house.people.map((p) => `${p.key}:${p.name}:${p.activity}:${p.queue}:${p.cached}`), dark].join(
    ' ',
  );
/** The last drawings, by what they show, so an office is there at once when it comes back or a screen lights again; the oldest goes past
 *  MOST. */
const drawings = new Map<string, Drawn>();
const MOST = 8;

/** The room drawn with its people, as image URLs and where each person and desk is; laid out and drawn again only when what it shows
 *  changes, not on each poll. `dark`: the keys, comma-joined, whose screens stay dark till they sit. */
export function useDrawn(house: House, dark = ''): Drawn | null {
  const sig = sigOf(house, dark);
  const lay = () => ({ sig, room: fittedRoom(house, new Set(dark ? dark.split(',') : [])) });
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
function drawOffice({ room, sig }: { room: Room; sig: string }): Drawn {
  const kept = drawings.get(sig);
  if (kept) {
    drawings.delete(sig);
    drawings.set(sig, kept);
    return kept;
  }
  renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
  // Both drawn through the same camera, which the room's size alone sets.
  const frame = drawRoom(renderer, { ...room, blocks: room.blocks.filter((b) => !inFront(b)) });
  const pic = shade(frame);
  const wallFrame = drawRoom(renderer, { ...room, blocks: room.blocks.filter(inFront) });
  const wall = shade(wallFrame);
  const cast = castOf(pic, frame, room);
  const top = topsOf(wall);
  // The founder walks round the front wall too, as the people do.
  const screens = screensOf(pic, frame);
  const ground = {
    ...groundOf(pic, { ...frame, solid: [...frame.solid, ...wallFrame.solid] }, room, { speed: 1.4, stride: 0.16 }),
    screens,
  };
  const drawn = {
    url: urlOf(pic),
    w: pic.w,
    h: pic.h,
    cast,
    ground,
    front: { url: urlOf(wall), pic: wall, door: boxOf(wall, wallFrame, DOOR), top, people: cast.figures.some((f) => touches(top, f.box)) },
    // Hinged by the corner, it opens into the room, away from the viewer.
    leaves: leavesOf(room.blocks, DOOR, 'right'),
    screens,
    stage: {
      seats: seatsOf(room, ground.floor),
      door: doorOf(room),
      floor: ground.floor,
      people: room.people,
      layout: room.w,
    },
  };
  drawings.set(sig, drawn);
  for (const old of drawings.keys()) if (drawings.size > MOST) drawings.delete(old);
  return drawn;
}
