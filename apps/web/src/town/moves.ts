// How the people of an office get from one place to another: each has a seat (where it is drawn: a desk chair, a sofa, by the door) and a
// free spot on the floor it gets up to; when its seat changes it walks up from the old one, along the floor and down into the new one.
// One coming in or going away walks through the door. Nobody ever gets from one place to another by a jump.
import { seatName, type Place, type Room } from '#src/town/room.ts';
import { nearestFree, pathTo, type Floor, type Spot } from '#src/town/walk.ts';

/** Where a person is: `id` names the place (room.ts seatName); `at` is where its figure is on the floor, `stand`
 *  the free floor it gets up to. */
export type Seat = { id: string; at: Spot; stand: Spot };
/** The door from inside: `inside` the floor just within it, `out` the doorway, where one coming or going is behind the door. */
export type Door = { inside: Spot; out: Spot };
/** A person whose seat changed: from where to where, null for out of the door. */
export type Change = { key: string; from: Seat | null; to: Seat | null };

/** How far in front of a seat one stands up: behind a desk chair, clear of a sofa's front. */
const UP = { chair: 0.5, sofa: 0.45 };

export function seatOf(place: Place, floor: Floor): Seat {
  const at: Spot = [place.x, place.z];
  const front: Spot = place.how === 'door' ? at : [place.x, place.z + (place.how === 'desk' ? UP.chair : UP.sofa)];
  return { id: seatName(place), at, stand: nearestFree(floor, front) ?? front };
}

/** Each placed person's seat, by key. */
export const seatsOf = (room: Room, floor: Floor) => new Map(room.places.map((p) => [p.person.key, seatOf(p, floor)]));

export const doorOf = (room: Room): Door => ({ inside: room.entry, out: [room.entry[0], room.d - 0.03] });

/** Whose seat differs between two layouts of the same room, one that is only in one of them coming in or going away. */
export function changesOf(was: ReadonlyMap<string, Seat>, now: ReadonlyMap<string, Seat>): Change[] {
  const keys = new Set([...was.keys(), ...now.keys()]);
  return [...keys].map((key) => ({ key, from: was.get(key) ?? null, to: now.get(key) ?? null })).filter((c) => c.from?.id !== c.to?.id);
}

/** The turns of the walk from seat `from` to seat `to` (null: the door), starting where `from` is: up to standing, the floor's shortest
 *  way, down into the seat. A way the floor does not have is walked straight. */
export function routeOf(floor: Floor, door: Door, from: Seat | null, to: Seat | null): Spot[] {
  const [a, b] = [from ?? { at: door.out, stand: door.inside }, to ?? { at: door.out, stand: door.inside }];
  const way = pathTo(floor, a.stand, b.stand) ?? [b.stand];
  const turns = [a.at, a.stand, ...way, b.at];
  return turns.filter((p, i) => i === 0 || p[0] !== turns[i - 1][0] || p[1] !== turns[i - 1][1]);
}

/** A seat to walk on from for one stopped mid-walk at `at`: where it is, and the free floor nearest it. */
export const standingAt = (floor: Floor, at: Spot): Seat => ({ id: '', at, stand: nearestFree(floor, at) ?? at });
