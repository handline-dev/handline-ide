// What a task asks of the founder: their open asks on it.
import { isPerson } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';

/** The founder's requests on this task still open. */
export const openAsks = (t: TaskDetail, me: string) =>
  (t.requests ?? []).filter((r) => (r.to === 'human' || isPerson(r.to, me)) && (r.state === 'queued' || r.state === 'landed'));
