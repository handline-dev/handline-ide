// Town mode: the town over the whole app in place of the sidebar and the tabs (the status row stays, as under Settings), opened from the
// sidebar's Town and left by Leave in its top strip, which also counts who is at work, waiting, stopped and on hold (StatusBar.tsx). The
// founder works from inside it: chats float over it, so it needs no tab.
import { AnimatePresence, motion } from 'framer-motion';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { CONTROLS_STRIP, useWindowLayer } from '#src/layout/windowLayer.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { leaveTown, useTownOpen } from '#src/town/history.ts';
import { TownStatus } from '#src/town/StatusBar.tsx';
import { TownTab } from '#src/town/Town.tsx';
import { ShapeButton } from '#src/town/List.tsx';

export function TownMode() {
  const shown = useTownOpen();
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
        <span className="app-no-drag flex items-center gap-0.5">
          <IconButton label="Back" onClick={() => history.back()}>
            <ChevronLeft size={16} />
          </IconButton>
          <IconButton label="Forward" onClick={() => history.forward()}>
            <ChevronRight size={16} />
          </IconButton>
        </span>
        <span className="font-medium">Town</span>
        {/* On a phone's width the list says each team's counts. */}
        <span className="flex min-w-0 max-sm:hidden">
          <TownStatus />
        </span>
        <span className="flex-1" />
        <ShapeButton />
        <button type="button" onClick={leaveTown} className="app-no-drag rounded-md px-1.5 py-0.5 text-muted hover:bg-hover hover:text-fg">
          Leave
        </button>
      </div>
      <div className="relative flex min-h-0 flex-1">
        <TownTab />
      </div>
    </>
  );
}
