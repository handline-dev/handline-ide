/** Picking tabs in one bar: a click picks one, Cmd/Ctrl+click adds or drops one, Shift+click picks the run from the last clicked one. */
export type Pick = { picked: string[]; anchor: string | null };
export type Mods = { add: boolean; range: boolean };

export const noPick: Pick = { picked: [], anchor: null };

/** The pick after a click on `id`, with `ids` the bar's tabs in order and `active` the one shown; `show` says to bring `id` up. */
export function clickPick(p: Pick, ids: string[], active: string | null, id: string, m: Mods): Pick & { show: boolean } {
  if (m.range) {
    const from = ids.indexOf(p.anchor ?? active ?? id);
    const to = ids.indexOf(id);
    const [a, b] = from < 0 ? [to, to] : [Math.min(from, to), Math.max(from, to)];
    return { picked: ids.slice(a, b + 1), anchor: p.anchor ?? active, show: true };
  }
  if (m.add) {
    const start = p.picked.length ? p.picked : active ? [active] : [];
    const picked = start.includes(id) ? start.filter((x) => x !== id) : ids.filter((x) => x === id || start.includes(x));
    return { picked, anchor: id, show: false };
  }
  return { picked: [], anchor: id, show: true };
}

/** What a drag from `id` carries: the pick when `id` is in it and it holds more than one, else `id` alone. */
export const dragged = (p: Pick, id: string): string[] => (p.picked.length > 1 && p.picked.includes(id) ? p.picked : [id]);
