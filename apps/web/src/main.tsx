import { setPageGo } from '#src/layout/openTab.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Connect } from '#src/connection/Connect.tsx';
import { Boundary, brokenView } from '#src/ui/Boundary.tsx';
import { Gallery } from '#src/Gallery.tsx';
import { instant, spring } from '#src/motion.ts';
import { Sidebar, SidebarControls, sidebarWidth } from '#src/Sidebar.tsx';
import { ModalHost } from '#src/ui/modal.tsx';
import { ToastHost } from '#src/ui/toast.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { useAgentLayouts } from '#src/layout/agentLayouts.ts';
import { Workspace } from '#src/Workspace.tsx';
import { StatusRow } from '#src/sidebar/StatusRow.tsx';
import { Settings } from '#src/settings/Settings.tsx';
import { openSettings } from '#src/settings/open.ts';
import '#src/styles.css';
import { trackShellInsets } from '#src/shellInsets.ts';
import { TerminalWindow, terminalWindowAgent } from '#src/tools/terminalWindow.tsx';

const clampWidth = (px: number) => Math.min(sidebarWidth.max, Math.max(sidebarWidth.min, px));

function App() {
  const [open, setOpen] = useState(true);
  const [width, setWidth] = useState(sidebarWidth.initial);
  const [dragging, setDragging] = useState(false);
  const { agent, page, layout, select, selectPage, dispatch } = useAgentLayouts();
  const root = useRef<HTMLDivElement>(null);
  // Settings covers the app instead of taking a tab.
  const goPage = (kind: string, title: string) => (kind === 'preferences' ? openSettings() : selectPage(kind, title));
  setPageGo(goPage);
  const toggle = () => setOpen((o) => !o);

  return (
    <>
      {/* Fixed top-left; right of the Mac window buttons in the shell (shellInsets.ts). */}
      <div className="app-no-drag fixed z-30" style={{ top: 'var(--controls-top, 6px)', left: 'calc(8px + var(--lights-w, 0px))' }}>
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
            <div className="app-drag shrink-0" style={{ height: 'max(40px, calc(var(--controls-top, 6px) + 34px))' }} />
            <Sidebar
              width={width}
              agent={agent}
              page={page}
              onSelectPage={goPage}
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
                    <div style={{ width: 'calc(104px + var(--lights-w, 0px))' }} />
                  </motion.div>
                )}
              </AnimatePresence>
            }
          />
        </div>
        <StatusRow />
      </div>
      <Settings />
    </>
  );
}

// Files dropped where nothing takes them are dropped, not opened in place of the app.
for (const type of ['dragover', 'drop'] as const)
  window.addEventListener(type, (e) => e.dataTransfer?.types.includes('Files') && e.preventDefault());

trackShellInsets();
const terminalOf = terminalWindowAgent(location.hash);
createRoot(document.getElementById('root')!).render(
  location.pathname === '/gallery' ? (
    <>
      <Gallery />
      <ModalHost />
      <ToastHost />
    </>
  ) : (
    <Connect>
      <Boundary loading={null} failed={brokenView('Handline')}>
        {terminalOf ? <TerminalWindow agent={terminalOf} /> : <App />}
      </Boundary>
      <ModalHost />
      <ToastHost />
    </Connect>
  ),
);
