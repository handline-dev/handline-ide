import type { Account } from '@handline/client';

/** The account's email, kept after the login drops; nothing when none. `as` writes "as <email>" for beside Log in. */
export function AccountEmail({ account, as, className = '' }: { account: Pick<Account, 'email'>; as?: boolean; className?: string }) {
  if (!account.email) return null;
  return (
    <span className={`truncate text-xs text-muted ${className}`} title={account.email}>
      {as ? `as ${account.email}` : account.email}
    </span>
  );
}
