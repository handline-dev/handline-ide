import { test } from 'node:test';
import assert from 'node:assert/strict';
import { folderIsHere, OPEN_IN_APPS } from '#src/index.ts';

const master = { id: 'n-master', self: true, device: null };
const lent = { id: 'n-lent', self: false, device: 'd-lent' };
const nodes = [master, lent];

test("on the master's own Mac the agents of the daemon's machine are here, a lent machine's are not", () => {
  assert.equal(folderIsHere({ daemonHere: false, device: undefined, nodes, agentNode: 'n-master' }), true);
  assert.equal(folderIsHere({ daemonHere: false, device: undefined, nodes, agentNode: 'n-lent' }), false);
});

test('an agent with no session yet, or a daemon that lists no machines, is on the one Mac there is', () => {
  assert.equal(folderIsHere({ daemonHere: false, device: undefined, nodes, agentNode: null }), true);
  assert.equal(folderIsHere({ daemonHere: false, device: undefined, nodes: [], agentNode: 'n-master' }), true);
  assert.equal(folderIsHere({ daemonHere: false, device: undefined, nodes: [], agentNode: undefined }), true);
});

test('on a lent Mac only the agents on its own node are here', () => {
  assert.equal(folderIsHere({ daemonHere: false, device: 'd-lent', nodes, agentNode: 'n-lent' }), true);
  assert.equal(folderIsHere({ daemonHere: false, device: 'd-lent', nodes, agentNode: 'n-master' }), false);
  assert.equal(folderIsHere({ daemonHere: false, device: 'd-lent', nodes, agentNode: null }), false);
});

test("a window on the daemon's own Mac that connects as a paired device sees that machine's agents as here", () => {
  assert.equal(folderIsHere({ daemonHere: true, device: 'd-desktop', nodes, agentNode: 'n-master' }), true);
  assert.equal(folderIsHere({ daemonHere: true, device: 'd-desktop', nodes, agentNode: null }), true);
  assert.equal(folderIsHere({ daemonHere: true, device: 'd-desktop', nodes, agentNode: 'n-lent' }), false);
});

test("a paired device that lends no machine holds no agent's folder", () => {
  assert.equal(folderIsHere({ daemonHere: false, device: 'd-phone', nodes, agentNode: 'n-master' }), false);
  assert.equal(folderIsHere({ daemonHere: false, device: 'd-phone', nodes: [], agentNode: null }), false);
});

test('the menu lists the three apps the shell opens, the editor first', () => {
  assert.deepEqual(
    OPEN_IN_APPS.map((a) => a.app),
    ['vscode', 'terminal', 'claude'],
  );
});
