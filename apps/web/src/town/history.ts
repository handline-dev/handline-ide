// The town in the browser's history, as the main screen's layout is (layout/screenLayout.ts): an entry carries where the founder is in
// the town (the office, or the city) and its windows. Opening the town, entering or leaving an office and bringing up a tab are steps
// Back and Forward walk; any other change rewrites the entry it is on. An entry without the town is outside it: Back to one leaves the
// town, Forward to a town entry comes back in.
import { useEffect, useRef } from 'react';
import { createStore } from '#src/persist/store.ts';
import type { Windows } from '#src/town/windows.ts';

export type Place = { house: string | null; windows: Windows };
type Entry = { town?: Place; modal?: boolean } & Record<string, unknown>;

const entry = (): Entry => (history.state as Entry | null) ?? {};
const placeIn = (s: unknown): Place | undefined => (s as Entry | null)?.town;

const open = createStore(false);
/** The place a Back or Forward brought: the town on screen takes it, and a town opening takes it as its start. */
const restored = createStore<Place | null>(null);
/** The next town change is a step of its own, not a rewrite of the entry it is on. */
const next = { push: false };

// A reload starts outside the town: the entry it lands on leaves the town too, so a later Back to it does not open it unasked.
if (entry().town) {
  const { town: _town, ...rest } = entry();
  history.replaceState(rest, '');
}

export const useTownOpen = open.useValue;
export function openTown() {
  if (open.get()) return;
  next.push = true;
  restored.set(null);
  open.set(true);
}
/** Leaves the town as a step: Back comes back in. */
export function leaveTown() {
  if (!open.get()) return;
  const { town: _town, modal: _modal, ...rest } = entry();
  history.pushState(rest, '');
  open.set(false);
}
/** The town's next change is a step Back returns from. */
export const stepTown = () => {
  next.push = true;
};
addEventListener('popstate', (e) => {
  const place = placeIn(e.state);
  if (!place) return open.set(false);
  restored.set(place);
  open.set(true);
});

/** Keeps the town's `place` in the history entry, and calls `restore` with the place a Back or Forward brings. */
export function useTownHistory(place: Place, restore: (p: Place) => void) {
  const json = JSON.stringify(place);
  const last = useRef<string | null>(null);
  const back = restored.useValue();
  const taken = useRef<Place | null>(null);
  // A restore asked and not drawn yet: the entry is not rewritten with what is about to go.
  const pending = useRef(false);
  useEffect(() => {
    if (!back || back === taken.current) return;
    taken.current = back;
    last.current = JSON.stringify(back);
    pending.current = true;
    restore(back);
  }, [back, restore]);
  // Every render: a step that changed nothing, or a restore of the place already shown, is used up here too.
  useEffect(() => {
    if (json === last.current) {
      pending.current = false;
      next.push = false;
      return;
    }
    // Fading out after a leave: the entry on screen is outside the town.
    if (pending.current || !open.get()) return;
    last.current = json;
    const push = next.push;
    next.push = false;
    const town = JSON.parse(json) as Place;
    if (!push) return history.replaceState({ ...entry(), town }, '');
    // A modal's mark stays on its own entry (ui/backCloses.ts).
    const { modal: _modal, ...rest } = entry();
    history.pushState({ ...rest, town }, '');
  });
}
