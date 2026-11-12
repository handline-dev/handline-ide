// Settings: over the whole app so the founder can focus on it, its sections in a menu on the left; Back at the menu's top, or Escape, leaves.
// The rest of legacy's Settings (schedules, saved answers, appearance) joins the menu as it is built.
import { AnimatePresence, motion } from 'framer-motion';
import { ArrowLeft, HardDrive, MonitorSmartphone, UserRound } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { DevicesTab } from '#src/settings/DevicesTab.tsx';
import { MachineTab } from '#src/settings/MachineTab.tsx';
import { AccountsTab } from '#src/usage/AccountsTab.tsx';
import { usePersisted } from '#src/persist/kv.ts';
import { CONTROLS_STRIP, LayerDragStrip, useWindowLayer } from '#src/layout/windowLayer.tsx';
import { closeSettings, settingsTarget, useSettingsOpen } from '#src/settings/open.ts';

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
          className="fixed inset-x-0 top-0 bottom-7 z-40 flex bg-bg"
        >
          <Frame />
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function Frame() {
  useWindowLayer(true, { drag: true });
  const [to] = useState(settingsTarget);
  const [saved, save] = usePersisted<Section>('settings:section', 'accounts');
  // A link's section wins until the menu is used.
  const [picked, pick] = useState(to.section);
  const section = picked ?? saved;
  const setSection = (s: Section) => (pick(s), save(s));
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !e.defaultPrevented && closeSettings();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);
  return (
    <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
      {/* Over the menu, not above it: its grey runs up to the window's top edge. */}
      <LayerDragStrip className="absolute left-0 top-0 w-56" />
      <div className="flex min-h-0 flex-1">
        <nav
          aria-label="Settings"
          className="flex w-56 shrink-0 flex-col gap-0.5 bg-surface px-2"
          style={{ paddingTop: `calc(${CONTROLS_STRIP} + 6px)` }}
        >
          {/* Where the sidebar's top was, as Linear and Mattermost do it: out from the corner you came in by. */}
          <button
            type="button"
            onClick={closeSettings}
            className="mb-4 flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-muted hover:bg-hover/30 hover:text-fg"
          >
            <ArrowLeft size={14} />
            Back
          </button>
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
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          {/* Empty tab bar, matching the app's. */}
          <LayerDragStrip height="40px" className="shrink-0 border-b border-border bg-surface" />
          <main className="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto pt-4">
            {section === 'accounts' && <AccountsTab focus={to.account} />}
            {section === 'machine' && <MachineTab />}
            {section === 'devices' && <DevicesTab />}
          </main>
        </div>
      </div>
    </div>
  );
}
