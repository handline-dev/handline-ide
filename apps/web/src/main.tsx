import { primaryMod } from '#src/ui/keys.ts';
import { setAgentGo, setPageGo } from '#src/layout/openTab.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Connect } from '#src/connection/Connect.tsx';
import { InCluster, SpaceCluster, splitSpace, useJoined, useJoinedReady } from '#src/connection/clusters.tsx';
import { Boundary, brokenView } from '#src/ui/Boundary.tsx';
import { Gallery } from '#src/Gallery.tsx';
import { WebPages } from '#src/tools/webPages.tsx';
import { InstallPoll } from '#src/install/install.tsx';
import { instant, spring } from '#src/motion.ts';
import { Sidebar, SidebarControls, sidebarWidth } from '#src/Sidebar.tsx';
import { ModalHost } from '#src/ui/modal.tsx';
import { ZoneHighlight } from '#src/ui/dnd.tsx';
import { OpenMenuHost } from '#src/layout/openable.tsx';
import { ToastHost } from '#src/ui/toast.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { useScreenLayout } from '#src/layout/screenLayout.ts';
import { Workspace } from '#src/Workspace.tsx';
import { openTown, TownMode } from '#src/town/mode.tsx';
import { StatusRow } from '#src/sidebar/StatusRow.tsx';
import { Onboarding } from '#src/onboarding/Onboarding.tsx';
import { Settings } from '#src/settings/Settings.tsx';
import { openSettings } from '#src/settings/open.ts';
import '#src/styles.css';
import '#src/theme.ts';
import { trackShellInsets } from '#src/shellInsets.ts';

const clampWidth = (px: number) => Math.min(sidebarWidth.max, Math.max(sidebarWidth.min, px));

function App() {
  const [open, setOpen] = useState(true);
  const [width, setWidth] = useState(sidebarWidth.initial);
  const [dragging, setDragging] = useState(false);
  const { agent, onScreen, layout, focus, setFocus, select, selectPage, dispatch, reopenClosed } = useScreenLayout();
  const root = useRef<HTMLDivElement>(null);
  // A joined cluster's agent opens in its own cluster.
  const here = splitSpace(agent ?? '');
  // A cluster left while one of its agents was on screen: Requests instead.
  const joined = useJoined();
  const joinedKnown = useJoinedReady();
  const gone = joinedKnown && !!here.cluster && !joined.some((j) => j.id === here.cluster);
  useEffect(() => {
    if (gone) selectPage('requests', 'Requests');
  }, [gone, selectPage]);
  // Settings and Town cover the app instead of taking a tab.
  const goPage = (kind: string, title: string) =>
    kind === 'preferences' ? openSettings() : kind === 'town' ? openTown() : selectPage(kind, title);
  setPageGo(goPage);
  setAgentGo(select);
  const toggle = () => setOpen((o) => !o);
  // Cmd+B (Ctrl+B off a Mac) toggles the sidebar; captured so a terminal does not take it, but left to the editor's bold.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== 'b' || e.altKey || e.shiftKey || !primaryMod(e)) return;
      if (e.target instanceof Element && e.target.closest('.cm-editor')) return;
      e.preventDefault();
      setOpen((o) => !o);
    };
    window.addEventListener('keydown', onKey, true);
    return () => window.removeEventListener('keydown', onKey, true);
  }, []);

  return (
    <>
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
              onScreen={onScreen}
              onSelectPage={goPage}
              onSelectAgent={select}
              onAgentSettings={(name) => openSettings({ section: 'agents', ...splitSpace(name) })}
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
          <InCluster key={here.cluster} id={here.cluster}>
            <SpaceCluster.Provider value={here.cluster}>
              <Workspace
                root={layout}
                agent={agent === null ? null : here.agent}
                focus={focus}
                onFocus={setFocus}
                dispatch={dispatch}
                reopenClosed={reopenClosed}
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
            </SpaceCluster.Provider>
          </InCluster>
        </div>
        <StatusRow />
      </div>
      <Settings />
      <TownMode />
      {/* Fixed top-left; right of the Mac window buttons in the shell (shellInsets.ts). Last in the page: the shell
          takes the last region drawn where drag strips and this overlap. */}
      <div className="app-no-drag fixed z-30" style={{ top: 'var(--controls-top, 6px)', left: 'calc(8px + var(--lights-w, 0px))' }}>
        <SidebarControls onToggle={toggle} />
      </div>
    </>
  );
}

// Files dropped where nothing takes them are dropped, not opened in place of the app.
for (const type of ['dragover', 'drop'] as const)
  window.addEventListener(type, (e) => e.dataTransfer?.types.includes('Files') && e.preventDefault());

trackShellInsets();
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
        <App />
        <WebPages />
        <InstallPoll />
        <Onboarding />
      </Boundary>
      <OpenMenuHost />
      <ZoneHighlight />
      <ModalHost />
      <ToastHost />
    </Connect>
  ),
);
