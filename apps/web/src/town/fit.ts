// An office's room shaped like a 4:3 screen: the bench length whose room comes nearest, then floor added across or in depth until the
// picture is 4:3. Worked out from the team's size alone, so desks stay put while agents come and go from them.
import type { House } from '#src/town/model.ts';
import { roomOf, type Room } from '#src/town/room.ts';
import { viewOf } from '#src/town/view.ts';

const ASPECT = 4 / 3;
const COLS = [2, 3, 4, 5, 6, 7, 8];

const aspectOf = (room: Room) => {
  const v = viewOf(room);
  return v.w / v.h;
};
const off = (room: Room) => Math.abs(Math.log(aspectOf(room) / ASPECT));

/** The room for a house, its picture 4:3. */
export function fittedRoom(house: House): Room {
  // Everyone resting on the sofas, the most room it can need: the shape is the team's size's, not who rests where right now.
  const base: House = { ...house, lead: null, people: house.people.map((p) => ({ ...p, activity: 'resting', cached: false })) };
  const cols = COLS.reduce((a, b) => (off(roomOf(base, b)) < off(roomOf(base, a)) ? b : a));
  const fit = { w: 0, d: 0 };
  for (let k = 0, room = roomOf(base, cols); k < 4; k++, room = roomOf(base, cols, fit)) {
    const a = aspectOf(room);
    // The base's depth is the floor in every case, so fewer resting never makes the room shallower.
    fit.d = Math.max(fit.d, room.d);
    if (a < ASPECT) fit.w += room.w * (ASPECT / a - 1);
    else fit.d = room.d * (a / ASPECT);
  }
  return roomOf(house, cols, fit);
}
