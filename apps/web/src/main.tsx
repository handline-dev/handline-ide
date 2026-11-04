import { AnimatePresence, motion } from 'framer-motion';
import { useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Connect } from '#src/connection/Connect.tsx';
import { Gallery } from '#src/Gallery.tsx';
import { instant, spring } from '#src/motion.ts';
import { Sidebar, SidebarControls, sidebarWidth } from '#src/Sidebar.tsx';
import { ModalHost } from '#src/ui/modal.tsx';
import { ToastHost } from '#src/ui/toast.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { StatusBar } from '#src/StatusBar.tsx';
import { useAgentLayouts } from '#src/layout/agentLayouts.ts';
import { Workspace } from '#src/Workspace.tsx';
import '#src/styles.css';

const clampWidth = (px: number) => Math.min(sidebarWidth.max, Math.max(sidebarWidth.min, px));

function App() {
  const [open, setOpen] = useState(true);
  const [width, setWidth] = useState(sidebarWidth.initial);
  const [dragging, setDragging] = useState(false);
  const { agent, page, layout, select, selectPage, dispatch } = useAgentLayouts();
  const root = useRef<HTMLDivElement>(null);
  const toggle = () => setOpen((o) => !o);

  return (
    <>
      {/* The controls never move: the top-left corner is the sidebar's header when it is open and the
          first tab bar's start when it is closed, where a spacer opens up for them. */}
      <div className="fixed top-1.5 left-2 z-30">
        <SidebarControls onToggle={toggle} />
      </div>
      <div className="flex h-full flex-col">
        <div ref={root} className="flex min-h-0 flex-1">
          <motion.div
            initial={false}
            animate={{ width: open ? width : 0 }}
            transition={dragging ? instant : spring}
            className="flex shrink-0 flex-col overflow-hidden bg-surface"
          >
            <div className="h-10 shrink-0" />
            <Sidebar
              width={width}
              agent={agent}
              page={page}
              onSelectPage={selectPage}
              onSelectAgent={select}
              onAgentSettings={(name) => {
                select(name);
                dispatch({ type: 'show', opens: 'settings', title: 'Settings' });
              }}
            />
          </motion.div>
          {open && (
            <Splitter
              axis="x"
              container={root}
              percent={((width - sidebarWidth.min) / (sidebarWidth.max - sidebarWidth.min)) * 100}
              onMove={(o) => setWidth(clampWidth(o))}
              onStep={(d) => setWidth((w) => clampWidth(w + d * 16))}
              onDragging={setDragging}
              label="Resize sidebar"
            />
          )}
          <Workspace
            root={layout}
            agent={agent}
            dispatch={dispatch}
            lead={
              <AnimatePresence initial={false}>
                {!open && (
                  <motion.div
                    key="lead"
                    initial={{ width: 0 }}
                    animate={{ width: 'auto' }}
                    exit={{ width: 0 }}
                    transition={spring}
                    className="shrink-0"
                  >
                    <div className="w-26" />
                  </motion.div>
                )}
              </AnimatePresence>
            }
          />
        </div>
        <StatusBar />
      </div>
    </>
  );
}

createRoot(document.getElementById('root')!).render(
  location.pathname === '/gallery' ? (
    <>
      <Gallery />
      <ModalHost />
      <ToastHost />
    </>
  ) : (
    <Connect>
      <App />
      <ModalHost />
      <ToastHost />
    </Connect>
  ),
);
