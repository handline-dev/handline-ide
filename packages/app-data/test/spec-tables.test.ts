// The tables the hooks run on come from the spec (contract/spec x-app-cache, x-app-write); the lists kept by hand must agree with it.
import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';
import { APP_WRITES } from '@handline/client';
import { NOT_FROM_UI, NOT_QUERIES, PAIRING, patches, writes } from '#src/index.ts';
import { NOT_CACHED, NOT_WRITTEN } from '#src/generated/reads.ts';

describe('the spec holds the app tables', () => {
  it('the reads the app does not cache are the NOT_QUERIES list', () => {
    expect([...NOT_CACHED].sort()).toEqual([...NOT_QUERIES].sort());
  });
  it('the writes the app does not make are the not-from-UI and pairing lists', () => {
    expect([...NOT_WRITTEN].sort()).toEqual([...NOT_FROM_UI, ...PAIRING].sort());
  });
  it('every patch is registered under a method the app writes', () => {
    expect(Object.keys(patches).filter((m) => !(m in writes))).toEqual([]);
  });
  it("each write the app makes has the spec's kind and label", () => {
    for (const [m, s] of Object.entries(writes)) expect({ kind: s.kind, label: s.label }, m).toEqual((APP_WRITES as Record<string, unknown>)[m]);
  });
});

describe('no cache key is written by hand', () => {
  it('a key is built from the method name: only reads.ts and the generated tables name a resource', () => {
    const src = path.resolve(import.meta.dirname, '../src');
    const files = fs.readdirSync(src, { recursive: true, encoding: 'utf8' }).filter((f) => f.endsWith('.ts') && f !== 'reads.ts' && !f.startsWith('generated'));
    const literal = /\[\s*'d'\s*,/;
    expect(files.filter((f) => literal.test(fs.readFileSync(path.join(src, f), 'utf8')))).toEqual([]);
  });
});
