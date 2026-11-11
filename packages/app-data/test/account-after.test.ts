import { describe, expect, it } from 'vitest';
import type { Account } from '@handline/client';
import { accountAfter } from '#src/account-after.ts';

const a = { name: 'x', label: 'x', cap: 4, cap_effective: 3, recover: true, paused: false } as unknown as Account;

describe('account.set paused', () => {
  it('a pause takes no new agent at once', () => {
    expect(accountAfter(a, { paused: true })).toMatchObject({ paused: true, cap_effective: 0 });
  });
  it('a cap change on a paused account keeps it at no new agent', () => {
    expect(accountAfter({ ...a, paused: true, cap_effective: 0 }, { cap: 6 })).toMatchObject({ cap: 6, cap_effective: 0 });
  });
  it('a resume leaves the cap in force to the refetch', () => {
    expect(accountAfter({ ...a, paused: true, cap_effective: 0 }, { paused: false })).toMatchObject({ paused: false, cap_effective: 0 });
  });
});
