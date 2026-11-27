// What a click on a Requests row does to the view beside the list. Pure.

/** The row clicked: its key, its task, and the comment it is about (a comment row). */
export type RowAim = { key: string; task: number | null; comment?: number };

/** A click on `row` while `shown` (the task beside the list) is up: a row of another task shows that task; a row of the task already
 * shown does not show it again but brings its own content into view, its comment or else the turn. Only the clicked row is painted. */
export function requestClick(
  shown: number | null,
  row: RowAim,
): { show: number | null; painted: string; scroll: { comment: number } | 'turn' | null } {
  if (row.task === null || row.task !== shown) return { show: row.task, painted: row.key, scroll: null };
  return { show: shown, painted: row.key, scroll: row.comment !== undefined ? { comment: row.comment } : 'turn' };
}

/** Whether a row is painted: only the one clicked, never a checked one nor another row of the same task. */
export const isPainted = (painted: string | null, key: string) => painted !== null && painted === key;
