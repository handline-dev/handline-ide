// Settings: its sections side by side in a switch at the top, Accounts first; the rest of legacy's Settings (devices, schedules, saved
// answers, appearance) comes into the same frame as it is built.
import { UserRound } from 'lucide-react';
import { AccountsTab } from '#src/usage/AccountsTab.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { Page } from '#src/ui/Page.tsx';

const SECTIONS = ['accounts'] as const;
type Section = (typeof SECTIONS)[number];
const WORD: Record<Section, string> = { accounts: 'Accounts' };
const ICON = { accounts: <UserRound size={12} /> };

export function Settings() {
  const [section, setSection] = usePersisted<Section>('settings:section', 'accounts');
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <Page className="pt-4">
        <Switch label="Settings" value={section} options={SECTIONS} word={(s) => WORD[s]} icon={(s) => ICON[s]} onChange={setSection} />
      </Page>
      {section === 'accounts' && <AccountsTab />}
    </div>
  );
}
