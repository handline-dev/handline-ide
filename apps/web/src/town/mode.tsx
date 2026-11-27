// Town mode: the town over the whole app in place of the sidebar and the tabs (the status row stays, as under Settings), opened from the
// sidebar's Town and left by Leave in its top strip, which also counts who is at work, waiting, stopped and on hold (StatusBar.tsx). The
// founder works from inside it: chats float over it, so it needs no tab.
import { AnimatePresence, motion } from 'framer-motion';
import { createStore } from '#src/persist/store.ts';
import { CONTROLS_STRIP, useWindowLayer } from '#src/layout/windowLayer.tsx';
import { useBackCloses } from '#src/ui/backCloses.ts';
import { TownStatus } from '#src/town/StatusBar.tsx';
import { TownTab } from '#src/town/Town.tsx';
import { ShapeButton } from '#src/town/List.tsx';

const open = createStore(false);
export const openTown = () => open.set(true);
const closeTown = () => open.set(false);
export const useTownOpen = open.useValue;

export function TownMode() {
  const shown = useTownOpen();
  useBackCloses(shown, closeTown);
  return (
    <AnimatePresence>
      {shown && (
        <motion.div
          key="town"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.12 }}
          className="fixed inset-x-0 top-0 bottom-7 z-40 flex flex-col bg-bg"
        >
          <Frame />
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function Frame() {
  useWindowLayer(true, { drag: true });
  return (
    <>
      {/* The window moves by this strip; its words sit right of the window buttons. */}
      <div
        className="app-drag layer-drag flex shrink-0 items-center gap-2 border-b border-border pr-2 text-sm"
        style={{ height: CONTROLS_STRIP, paddingLeft: 'calc(16px + var(--lights-w, 0px))' }}
      >
        <span className="font-medium">Town</span>
        {/* On a phone's width the list says each team's counts. */}
        <span className="flex min-w-0 max-sm:hidden">
          <TownStatus />
        </span>
        <span className="flex-1" />
        <ShapeButton />
        <button type="button" onClick={closeTown} className="app-no-drag rounded-md px-1.5 py-0.5 text-muted hover:bg-hover hover:text-fg">
          Leave
        </button>
      </div>
      <div className="relative flex min-h-0 flex-1">
        <TownTab />
      </div>
    </>
  );
}
