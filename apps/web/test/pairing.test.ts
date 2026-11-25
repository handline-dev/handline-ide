import { describe, expect, it } from 'vitest';
import { appLink, codeFrom, forgetToken, heldDevice, invite, loadToken, rememberCluster } from '#src/connection/pairing.ts';

const FP = 'ab'.repeat(32);

describe('a pairing link', () => {
  it('carries the code, and the fingerprint of a self-made certificate', () => {
    expect(invite('#pair=5S3K-R92R')).toEqual({ code: '5S3K-R92R', fp: null });
    expect(invite(`#pair=5S3K%2DR92R&fp=${FP.toUpperCase()}`)).toEqual({ code: '5S3K-R92R', fp: FP });
    expect(invite('#settings')).toBeNull();
    expect(invite('#pair=%E0%A4%A')).toBeNull();
  });

  it('opens in the app as the same listener and code', () => {
    expect(appLink('home.tail1.ts.net:7443', { code: 'A B', fp: null })).toBe('handline://pair?host=home.tail1.ts.net%3A7443&code=A%20B');
    expect(appLink('10.0.0.2:7443', { code: 'C', fp: FP })).toBe(`handline://pair?host=10.0.0.2%3A7443&code=C&fp=${FP}`);
  });

  it('pasted whole, a bare code or an old handline:// line gives the code', () => {
    expect(codeFrom(` https://home.tail1.ts.net:7443/#pair=5S3K&fp=${FP} `)).toBe('5S3K');
    expect(codeFrom('handline://pair?host=h&code=7Q2X')).toBe('7Q2X');
    expect(codeFrom(' 7Q2X ')).toBe('7Q2X');
  });
});

describe('a token per cluster', () => {
  const kv = () => {
    const m = new Map<string, string>();
    return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
  };
  const a = 'ws://100.64.0.1:7443/rpc';
  const b = 'ws://my-mac.ts.net:7443/rpc';
  it('another address the cluster listed as its door reuses the token; an address it never listed gets none', () => {
    const s = kv();
    s.setItem(`token:${a}`, 'tok');
    s.setItem(`device:${a}`, 'dev_1');
    rememberCluster(s, a, 'node-1', [a, b]);
    expect(loadToken(s, b)).toBe('tok');
    expect(loadToken(s, 'ws://evil.example:7443/rpc')).toBeNull();
    expect(heldDevice(s, b)).toBe('dev_1');
  });
  it('a refused token goes for the whole cluster, and the device it held stays to be replaced', () => {
    const s = kv();
    s.setItem(`token:${a}`, 'tok');
    s.setItem(`device:${a}`, 'dev_1');
    rememberCluster(s, a, 'node-1', [a, b]);
    forgetToken(s, b);
    expect(loadToken(s, a)).toBe('tok');
    expect(loadToken(s, b)).toBeNull();
    expect(heldDevice(s, b)).toBe('dev_1');
  });
});

describe('a token per cluster, as its doors change', () => {
  const kv = () => {
    const m = new Map<string, string>();
    return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
  };
  const a = 'ws://100.64.0.1:7443/rpc';
  const b = 'ws://my-mac.ts.net:7443/rpc';
  const c = 'ws://127.0.0.1:7443/rpc';
  it('a door the cluster no longer lists gets no token', () => {
    const s = kv();
    s.setItem(`token:${a}`, 'tok');
    rememberCluster(s, a, 'node-1', [a, b]);
    rememberCluster(s, a, 'node-1', [a]);
    expect(loadToken(s, b)).toBeNull();
  });
  it('an address that took the token from its cluster keeps the door list current', () => {
    const s = kv();
    s.setItem(`token:${a}`, 'tok');
    rememberCluster(s, a, 'node-1', [a, b]);
    rememberCluster(s, b, 'node-1', [b, c]);
    expect(loadToken(s, c)).toBe('tok');
  });
  it("a refused old token does not clear the cluster's newer one", () => {
    const s = kv();
    s.setItem(`token:${a}`, 'old');
    s.setItem(`token:${b}`, 'new');
    rememberCluster(s, b, 'node-1', [a, b]);
    forgetToken(s, a);
    expect(loadToken(s, b)).toBe('new');
  });
});
