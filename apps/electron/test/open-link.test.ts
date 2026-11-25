import assert from 'node:assert/strict';
import test from 'node:test';
import { OpenLinks, isPairLink } from '#src/open-link.ts';

test('a pairing link on a Tailscale name the app is opened with waits for the window, then each goes straight to it; anything else is dropped', () => {
  const l = new OpenLinks();
  const got: string[] = [];
  l.add('handline://pair?host=a.ts.net%3A7443&code=X');
  l.add('handline://other');
  l.add('https://a.ts.net:7443/#pair=X');
  l.add(`handline://pair?host=studio.evil.example%3A443&code=X&fp=${'ab'.repeat(32)}`); // any web page can open one
  l.add('handline://pair?code=X');
  l.serve((u) => got.push(u));
  assert.deepEqual(got, ['handline://pair?host=a.ts.net%3A7443&code=X']);
  l.add('handline://pair?host=b.ts.net%3A7443&code=Y');
  assert.deepEqual(got.at(-1), 'handline://pair?host=b.ts.net%3A7443&code=Y');
  assert.equal(isPairLink(`handline://pair?code=${'x'.repeat(2048)}`), false);
});
