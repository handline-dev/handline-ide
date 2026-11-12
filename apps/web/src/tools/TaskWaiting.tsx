// What a task asks of the founder: their open asks on it, and the choices a question lays out.
import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';

/** The founder's requests on this task still open. */
export const openAsks = (t: TaskDetail, me: string) =>
  (t.requests ?? []).filter((r) => (r.to === 'human' || isPerson(r.to, me)) && (r.state === 'queued' || r.state === 'landed'));

/** The choices an ask lays out, one a line: "A) ...", "B. ...", "1: ..."; fewer than two is no choice. */
const CHOICE = /^[ \t]*([A-D1-4])[).:][ \t]+(.+)$/gm;
export function choices(ask: string) {
  const found = [...ask.matchAll(CHOICE)].map((m) => ({ key: m[1]!, text: m[2]!.trim() }));
  return found.length >= 2 ? found : [];
}
