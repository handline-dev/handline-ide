import { describe, expect, it } from 'vitest';
import type { Account } from '@handline/client';
import { accountAfter } from '#src/account-after.ts';

const a = { name: 'x', label: 'x', recover: true, paused: false } as unknown as Account;

describe('account.set', () => {
  it('a pause and a resume show at once', () => {
    expect(accountAfter(a, { paused: true })).toMatchObject({ paused: true });
    expect(accountAfter({ ...a, paused: true }, { paused: false })).toMatchObject({ paused: false });
  });
  it('an empty alias is the name again; the fields not given stay', () => {
    expect(accountAfter({ ...a, label: 'Work' }, { label: '' })).toMatchObject({ label: 'x', recover: true });
  });
});
