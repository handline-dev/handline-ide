import type { Account } from '@handline/client';
import { edit } from '#src/cache.ts';
import { qk } from '#src/reads.ts';
import type { Patch } from '#src/write-spec.ts';

// The daemon sends no event for a removed account, so the confirmed removal takes the row out itself (a dry run changes nothing).
export const accountRemove: Patch = {
  reconcile: (c, p) => {
    const { account, dry_run } = p as { account: string; dry_run?: boolean };
    if (!dry_run)
      edit<{ accounts: Account[] }>(c.qc, qk.accounts(c.d), (l) => ({ ...l, accounts: l.accounts.filter((a) => a.name !== account) }));
  },
};
