import { accountLabel } from '@handline/app-core';
import { useAccounts } from '@handline/app-data';
import { openSettings } from '#src/settings/open.ts';

/** An account's screen name by its key: its label, the key itself while the list is not read or the account is gone. */
export function useAccountLabel(): (name: string) => string {
  const accounts = useAccounts().data?.accounts;
  return (name) => accountLabel(accounts, name);
}

/** An account as screens show it: its label, and a click opens that account in Settings. */
export function AccountName({ name, className = '' }: { name: string; className?: string }) {
  const label = useAccountLabel();
  return (
    <span
      role="link"
      tabIndex={0}
      onClick={(e) => {
        e.stopPropagation();
        openSettings({ section: 'accounts', account: name });
      }}
      onKeyDown={(e) => {
        if (e.key !== 'Enter') return;
        e.stopPropagation();
        openSettings({ section: 'accounts', account: name });
      }}
      className={`cursor-pointer hover:underline ${className}`}
    >
      {label(name)}
    </span>
  );
}
