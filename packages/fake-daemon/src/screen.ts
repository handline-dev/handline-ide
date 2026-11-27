// The fake daemon's screen captures: screen.history and screen.picture, over two fixed rows.
import type { Methods } from '@handline/contract';

/** A 1 px PNG. */
const PIXEL = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';

const history = (at: string): Methods['screen.history']['result'] => ({
  captures: [
    { id: 2, at, agent: 'alice', task: 7, app: 'Finder', title: 'Downloads', ok: true, status: 'ok', reason: '', has_picture: true },
    {
      id: 1,
      at,
      agent: 'bob',
      task: 0,
      app: 'Notes',
      title: '',
      ok: false,
      status: 'refused',
      reason: 'Block screen capture is on',
      has_picture: false,
    },
  ],
});

/** The answer to a capture read; null for any other method. */
export function screenCall(method: string, at: string): { value: unknown } | null {
  if (method === 'screen.history') return { value: history(at) };
  if (method === 'screen.picture') return { value: { mime: 'image/png', data: PIXEL } };
  return null;
}
