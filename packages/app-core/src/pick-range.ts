// A Shift-click on a row's check box in any list: every shown row from the last one clicked to this one takes the state the last
// one has (checked picks the range, unchecked clears it). Pure; the list passes the keys it draws, in their order.

/** The rows a Shift-click on `clicked` changes, and whether they are picked or cleared. `anchor` is the row clicked last; when it
 * is not shown any more (filtered out, under a folded section, gone) the click is a plain toggle of `clicked`. */
export function pickRange(
  shown: readonly string[],
  anchor: string | null,
  clicked: string,
  picked: (key: string) => boolean,
): { keys: string[]; on: boolean } {
  const to = shown.indexOf(clicked);
  const from = anchor === null ? -1 : shown.indexOf(anchor);
  if (from < 0 || to < 0) return { keys: [clicked], on: !picked(clicked) };
  const [a, b] = from < to ? [from, to] : [to, from];
  return { keys: shown.slice(a, b + 1), on: picked(anchor!) };
}
