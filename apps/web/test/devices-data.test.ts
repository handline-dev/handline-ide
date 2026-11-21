import { describe, expect, it } from 'vitest';
import type { Device, Node } from '@handline/client';
import { deviceRows, isStale } from '#src/settings/data/devices.ts';

const now = Date.parse('2026-11-06T00:00:00Z');
const day = 86_400_000;
const seen = (days: number) => new Date(now - days * day).toISOString();

describe('isStale', () => {
  it('is a device not connected and not seen for over 30 days', () => {
    expect(isStale({ connected: false, last_seen: seen(31) }, now)).toBe(true);
    expect(isStale({ connected: false, last_seen: seen(30) }, now)).toBe(false);
  });
  it('is never a connected device or one never seen', () => {
    expect(isStale({ connected: true, last_seen: seen(90) }, now)).toBe(false);
    expect(isStale({ connected: false, last_seen: null }, now)).toBe(false);
  });
});

const iso = (minsAgo: number) => new Date(now - minsAgo * 60_000).toISOString();
const dev = (id: string, o: Partial<Device> = {}): Device => ({
  id,
  name: id,
  role: 'owner',
  person: 'p1',
  created: iso(600),
  last_seen: iso(1),
  connected: true,
  push: false,
  ...o,
});
const node = (id: string, o: Partial<Node> = {}): Node => ({
  id,
  person: 'p1',
  name: id,
  joined_at: iso(600),
  last_seen: iso(0),
  master_number: 1,
  master: false,
  self: false,
  online: true,
  lending: 'run',
  ...o,
});
const master = node('n-master', { name: 'ron-mbp', master: true, self: true });
const opts = { me: 'p1', here: null, paused: false, now };

describe('deviceRows', () => {
  it("puts the master's own Mac first, on Run, Pause while the daemon is paused, and marks it here when the app is no paired device", () => {
    const [m] = deviceRows([], [master], [{ key: 'ui', node: 'n-master' }], opts);
    expect(m).toMatchObject({ id: 'n-master', master: true, here: true, mine: true, mode: 'run', state: 'running', agents: ['ui'] });
    expect(deviceRows([], [master], [], { ...opts, paused: true })[0]).toMatchObject({ mode: 'pause', state: 'paused' });
  });

  it("joins a Mac's device to its node: its mode, online, agents and what it lacks", () => {
    const studio = dev('d-studio', { kind: 'mac', lending: 'run' });
    const lacks = [{ agent: 'ui', why: 'needs pencil' }];
    const rows = deviceRows([studio], [master, node('n-studio', { device: 'd-studio', lacks })], [{ key: 'core', node: 'n-studio' }], opts);
    expect(rows[1]).toMatchObject({ id: 'd-studio', runner: true, mode: 'run', state: 'running', agents: ['core'], lacks });
  });

  it('tells offline, off, paused and starting apart, offline first', () => {
    const at = (lending: Device['lending'], n: Partial<Node> | null, o: Partial<Device> = {}) =>
      deviceRows([dev('d', { kind: 'mac', lending, ...o })], n ? [node('n', { device: 'd', ...n })] : [], [], opts)[0]!.state;
    expect(at('pause', { lending: 'pause' })).toBe('paused');
    expect(at('off', null)).toBe('off');
    expect(at('off', null, { connected: false })).toBe('offline');
    expect(at('run', { online: false, last_seen: iso(5) })).toBe('offline');
    expect(at('run', { online: false, last_seen: null }, { connected: false })).toBe('starting');
  });

  it('gives a phone or a browser no mode, and lets only its own person set a Mac', () => {
    const rows = deviceRows([dev('phone', { kind: 'ios' }), dev('bob-mac', { kind: 'mac', person: 'p2', lending: 'off' })], [], [], opts);
    expect(rows.find((r) => r.id === 'phone')).toMatchObject({ runner: false, mode: null, state: 'connected' });
    expect(rows.find((r) => r.id === 'bob-mac')).toMatchObject({ runner: true, mine: false });
  });
});
