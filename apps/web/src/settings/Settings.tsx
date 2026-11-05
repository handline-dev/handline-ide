// Settings: over the whole app so the founder can focus on it, its sections in a menu on the left, closed by X or Escape.
// The rest of legacy's Settings (schedules, saved answers, appearance) joins the menu as it is built.
import { AnimatePresence, motion } from 'framer-motion';
import { HardDrive, MonitorSmartphone, UserRound, X } from 'lucide-react';
import { useEffect, type ReactNode } from 'react';
import { AccountsTab } from '#src/usage/AccountsTab.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { closeSettings, useSettingsOpen } from '#src/settings/open.ts';

const SECTIONS = ['accounts', 'machine', 'devices'] as const;
type Section = (typeof SECTIONS)[number];
const MENU: Record<Section, { word: string; icon: ReactNode }> = {
  accounts: { word: 'Accounts', icon: <UserRound size={14} /> },
  machine: { word: 'Machine', icon: <HardDrive size={14} /> },
  devices: { word: 'Devices', icon: <MonitorSmartphone size={14} /> },
};

export function Settings() {
  const open = useSettingsOpen();
  return (
    <AnimatePresence>
      {open && (
        <motion.div
          key="settings"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.12 }}
          className="fixed inset-0 z-40 flex bg-bg"
        >
          <Frame />
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function Frame() {
  const [section, setSection] = usePersisted<Section>('settings:section', 'accounts');
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !e.defaultPrevented && closeSettings();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  return (
    <>
      <nav aria-label="Settings" className="flex w-56 shrink-0 flex-col gap-0.5 bg-surface px-2 pt-12">
        <div className="px-2 pb-2 text-xs text-muted">Settings</div>
        {SECTIONS.map((s) => (
          <button
            key={s}
            type="button"
            aria-current={section === s ? 'page' : undefined}
            onClick={() => setSection(s)}
            className={`flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm ${
              section === s ? 'bg-selected text-fg' : 'text-muted hover:bg-hover/30 hover:text-fg'
            }`}
          >
            {MENU[s].icon}
            {MENU[s].word}
          </button>
        ))}
      </nav>
      <main className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto pt-8">
        <div className="absolute top-2 right-2">
          <IconButton label="Close settings" onClick={closeSettings}>
            <X size={16} />
          </IconButton>
        </div>
        {section === 'accounts' && <AccountsTab />}
        {section !== 'accounts' && <div className="px-8 text-sm text-muted">{MENU[section].word} comes next.</div>}
      </main>
    </>
  );
}
