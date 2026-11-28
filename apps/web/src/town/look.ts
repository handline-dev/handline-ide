// The founder's own look, picked on this device (MyLook.tsx): its own figure, or one of the people on the agents' walking sheet. Kept in
// this browser's storage, so another device keeps its own.
import { useSyncExternalStore } from 'react';
import { read, write } from '#src/persist/kv.ts';
import { WALK, WALK_SHEET } from '#src/town/people.ts';
import { MY_CHAIR } from '#src/town/room.ts';

export const FOUNDER_SHEET = `${import.meta.env.BASE_URL}town/founder.png`;

/** A sheet of walking frames: its image, one cell's size and the whole size, in sheet pixels. Every sheet has nine columns: down, up and
 *  left, each standing, left foot, right foot. */
export type Sheet = { url: string; cell: readonly [number, number]; size: readonly [number, number] };
/** Seated in the founder's chair: its head starts `head` down its cell, and nothing of it under the seat, `seat` metres high. */
export type Sitting = { chair: string; head: number; seat: number };
/** A look: its sheet and row there, and how it sits. */
export type Look = { sheet: Sheet; row: number; sit: Sitting };

const OWN: Sheet = { url: FOUNDER_SHEET, cell: [144, 228], size: [1296, 228] };
export const WALKERS: Sheet = { url: WALK_SHEET, cell: WALK.cell, size: WALK.size };
const sitting = (top: number, cell: number): Sitting => ({ chair: MY_CHAIR, head: top / cell, seat: 0.25 });
/** Where each walking person's head starts facing away, in sheet pixels (from the sheet itself). */
const WALK_HEADS = [39, 51, 36, 27, 33, 45, 51, 45, 57, 24, 30, 15, 45];

/** The looks to pick from: the founder's own first, then each of the agents' people. */
export const LOOKS: Look[] = [
  { sheet: OWN, row: 0, sit: sitting(24, OWN.cell[1]) },
  ...WALK_HEADS.map((top, row) => ({ sheet: WALKERS, row, sit: sitting(top, WALKERS.cell[1]) })),
];

const KEY = 'town.look';
/** A kept pick that is no look (a sheet with fewer people now) is the founder's own. */
const valid = (n: unknown): number => (typeof n === 'number' && Number.isInteger(n) && n >= 0 && n < LOOKS.length ? n : 0);

const all = new Set<() => void>();
let picked: number | null = null;

/** The look picked on this device. */
export const lookNow = (): number => (picked ??= valid(read(KEY, 0)));

export function pickLook(n: number) {
  picked = valid(n);
  write(KEY, picked || undefined);
  all.forEach((l) => l());
}

const subscribe = (l: () => void) => (all.add(l), () => void all.delete(l));

/** The founder's look, drawn again when another is picked. */
export const useLook = (): Look => LOOKS[useSyncExternalStore(subscribe, lookNow)];
/** Which of LOOKS is picked. */
export const useLookIndex = (): number => useSyncExternalStore(subscribe, lookNow);
