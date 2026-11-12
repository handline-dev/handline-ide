import type { Account } from '@handline/client';

/** The account's email, kept after the login drops; nothing when none. `before` goes ahead of it, as "as" beside Log in. */
export function AccountEmail({
  account,
  before,
  className = '',
}: {
  account: Pick<Account, 'email'>;
  before?: string;
  className?: string;
}) {
  if (!account.email) return null;
  return (
    <span className={`truncate text-xs text-muted ${className}`} title={account.email}>
      {before ? `${before} ${account.email}` : account.email}
    </span>
  );
}
