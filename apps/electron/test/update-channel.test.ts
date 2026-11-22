import { test } from 'node:test';
import assert from 'node:assert/strict';
import { channelOf, isChannel, withChannel } from '#src/update-channel.ts';

const BASE = 'https://github.com/handline-dev/handline-ide/releases/download';

test('a build names its channel by the tag its update address ends in, and another channel is the same address with its tag', () => {
  assert.equal(channelOf(`${BASE}/mac-nightly`), 'nightly');
  assert.equal(channelOf(`${BASE}/mac-canary/`), 'canary');
  assert.equal(channelOf(`${BASE}/mac-stable`), 'stable');
  assert.equal(channelOf('https://example.org/dl'), null);
  assert.equal(withChannel(`${BASE}/mac-nightly`, 'stable'), `${BASE}/mac-stable`);
  assert.equal(withChannel('https://example.org/dl', 'stable'), 'https://example.org/dl');
  assert.equal(isChannel('canary'), true);
  assert.equal(isChannel('edge'), false);
});
