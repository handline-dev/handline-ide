import { test } from 'node:test';
import assert from 'node:assert/strict';
import { askQuit, askStop, guardQuit, quitDetail, quitQuietly, stopDetail, type Ask } from '#src/ask.ts';

/** An Ask that records what it showed and answers with the button given (and the checkbox). */
const answer = (response: number, checkboxChecked = false) => {
  const shown: Parameters<Ask>[0][] = [];
  const ask: Ask = async (o) => (shown.push(o), { response, checkboxChecked });
  return { ask, shown };
};

test('Quit asks while the daemon runs, says it and the working agents keep running, Quit the default', async () => {
  const a = answer(0);
  assert.deepEqual(await askQuit({ running: true, working: 3, skip: false, ask: a.ask }), { quit: true, dontAsk: false });
  const o = a.shown[0]!;
  assert.equal(
    o.detail,
    'Handline closes. The daemon and 3 working agents keep running in the background. Open Handline again to see them.',
  );
  assert.deepEqual(o.buttons, ['Quit', 'Cancel']);
  assert.equal(o.defaultId, 0);
  assert.equal(o.checkboxLabel, "Don't show again");
  assert.equal(quitDetail(0), 'Handline closes. The daemon keeps running in the background. Open Handline again to see it.');
  assert.deepEqual(await askQuit({ running: true, working: 1, skip: false, ask: answer(1).ask }), { quit: false, dontAsk: false });
});

test('Don’t show again is remembered from a Quit; once kept, and with no daemon running, nothing is asked', async () => {
  assert.deepEqual(await askQuit({ running: true, working: 2, skip: false, ask: answer(0, true).ask }), { quit: true, dontAsk: true });
  assert.deepEqual(await askQuit({ running: true, working: 2, skip: false, ask: answer(1, true).ask }), { quit: false, dontAsk: false });
  const kept = answer(1);
  assert.deepEqual(await askQuit({ running: true, working: 2, skip: true, ask: kept.ask }), { quit: true, dontAsk: false });
  const none = answer(1);
  assert.deepEqual(await askQuit({ running: false, working: 0, skip: false, ask: none.ask }), { quit: true, dontAsk: false });
  assert.equal(kept.shown.length + none.shown.length, 0);
});

test('Stop daemon always asks, Cancel the default, and names the agents mid-turn', async () => {
  const no = answer(1);
  assert.equal(await askStop({ working: 3, ask: no.ask }), false);
  assert.equal(no.shown[0]!.message, 'Stop the daemon?');
  assert.equal(
    no.shown[0]!.detail,
    '3 agents are mid-turn: they stop being served until the daemon starts again. Their work stays in their folders.',
  );
  assert.deepEqual(no.shown[0]!.buttons, ['Stop', 'Cancel']);
  assert.equal(no.shown[0]!.defaultId, 1);
  assert.equal(await askStop({ working: 0, ask: answer(0).ask }), true);
  assert.match(stopDetail(0), /No agent is mid-turn/);
});

test('every quit the person starts asks first; Cancel keeps the app; a quit the app makes itself asks nothing', async () => {
  let handler: ((e: { preventDefault(): void }) => void) | undefined;
  const quits: string[] = [];
  const app = {
    on: (_: 'before-quit', f: typeof handler) => void (handler = f),
    quit: () => (quits.push('quit'), handler!({ preventDefault: () => quits.push('prevented') })),
  };
  let reply = false;
  let asked = 0;
  guardQuit(app, async () => (asked++, reply));
  app.quit(); // Cmd-Q: asked, Cancel
  await new Promise((r) => setImmediate(r));
  assert.deepEqual(quits.splice(0), ['quit', 'prevented']);
  reply = true;
  app.quit(); // asked, Quit: the app quits for real
  await new Promise((r) => setImmediate(r));
  assert.deepEqual(quits.splice(0), ['quit', 'prevented', 'quit']);
  assert.equal(asked, 2);
  quitQuietly(app); // an update's relaunch
  assert.deepEqual(quits.splice(0), ['quit']);
  assert.equal(asked, 2);
});
