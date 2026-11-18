// A web tab's page lives here, in one layer over the app, not in the tab: a tab moved to another pane or workspace leaves and comes
// back as a new element, and a page in it would load again. The tab draws a slot; its page follows the slot's box each frame and
// waits hidden while no slot shows it. It goes when no layout has its tab any more (sweepTabState).
import { useEffect, useSyncExternalStore } from 'react';
import { shell } from '#src/shell.ts';

/** What the app calls on a <webview> (Electron's element). */
type Webview = HTMLElement & { goBack(): void; goForward(): void; reload(): void; getWebContentsId(): number };
/** What draws: the address, and for an iframe the reloads (a reload is a new iframe). */
type Page = { tab: string; url: string; reload: number };

const WEBVIEW = !!shell()?.webview;
const pages = new Map<string, Page>();
/** Read each frame, not drawn: the slots showing each tab (the newest last) and each page's element. */
const slots = new Map<string, HTMLElement[]>();
const els = new Map<string, HTMLElement>();
let list: Page[] = [];
const listeners = new Set<() => void>();
const changed = () => {
  list = [...pages.values()];
  for (const l of listeners) l();
};
const subscribe = (l: () => void) => (listeners.add(l), () => void listeners.delete(l));
const snapshot = () => list;

/** Drops the pages of tabs not among `tabs`. */
export function dropWebPages(tabs: ReadonlySet<string>) {
  const gone = [...pages.keys()].filter((t) => !tabs.has(t));
  for (const t of gone) pages.delete(t);
  if (gone.length) changed();
}

/** Back, forward, reload and the id to capture the tab's page by. Back and forward reach an iframe's history on the app's origin only. */
export function webPage(tab: string) {
  const el = () => els.get(tab) ?? null;
  return {
    go(go: 'back' | 'forward') {
      if (WEBVIEW) {
        const w = el() as Webview | null;
        return go === 'back' ? w?.goBack() : w?.goForward();
      }
      try {
        (el() as HTMLIFrameElement | null)?.contentWindow?.history.go(go === 'back' ? -1 : 1);
      } catch {
        /* another origin */
      }
    },
    reload() {
      const p = pages.get(tab);
      if (WEBVIEW) (el() as Webview | null)?.reload();
      else if (p) {
        pages.set(tab, { ...p, reload: p.reload + 1 });
        changed();
      }
    },
    /** Null for an iframe, or a webview not loaded yet. */
    id(): number | null {
      try {
        return WEBVIEW ? ((el() as Webview | null)?.getWebContentsId() ?? null) : null;
      } catch {
        return null;
      }
    },
  };
}

/** Where the tab's page shows: an empty box its page is laid over. */
export function WebSlot({ tab, url }: { tab: string; url: string }) {
  useEffect(() => {
    const p = pages.get(tab);
    if (p?.url !== url) {
      pages.set(tab, { tab, url, reload: p?.reload ?? 0 });
      changed();
    }
  }, [tab, url]);
  const ref = (box: HTMLDivElement | null) => {
    if (!box) return;
    slots.set(tab, [...(slots.get(tab) ?? []), box]);
    return () =>
      void slots.set(
        tab,
        (slots.get(tab) ?? []).filter((b) => b !== box),
      );
  };
  return <div ref={ref} className="min-h-0 flex-1 bg-bg" />;
}

/** While a tab is dragged, the pages let it through to the drop places under them. */
let dragging = false;

/** Lays each page over the newest slot of its tab on screen, or hides it; one loop for every page, writing only what changed. */
function place(last: Map<string, string>) {
  for (const [tab, el] of els) {
    const slot = slots
      .get(tab)
      ?.filter((b) => b.isConnected)
      .at(-1);
    const r = slot?.getBoundingClientRect();
    const shown = !!r && r.width > 0 && r.height > 0;
    const key = shown ? `${r.left},${r.top},${r.width},${r.height},${dragging}` : 'off';
    if (last.get(tab) === key) continue;
    last.set(tab, key);
    Object.assign(el.style, {
      visibility: shown ? 'visible' : 'hidden',
      pointerEvents: shown && !dragging ? 'auto' : 'none',
      ...(shown ? { left: `${r.left}px`, top: `${r.top}px`, width: `${r.width}px`, height: `${r.height}px` } : {}),
    });
  }
}

/** The layer of every web tab's page, once at the app's root. */
export function WebPages() {
  const all = useSyncExternalStore(subscribe, snapshot);
  useEffect(() => {
    const on = () => (dragging = true);
    // A drag that ends outside the window may send no dragend: the next press or leaving the window ends it.
    const off = () => (dragging = false);
    const ons = [
      ['dragstart', on],
      ['dragend', off],
      ['drop', off],
      ['pointerdown', off],
      ['blur', off],
    ] as const;
    for (const [e, f] of ons) addEventListener(e, f, true);
    const last = new Map<string, string>();
    let frame = 0;
    const loop = () => {
      place(last);
      frame = requestAnimationFrame(loop);
    };
    loop();
    return () => {
      cancelAnimationFrame(frame);
      for (const [e, f] of ons) removeEventListener(e, f, true);
    };
  }, []);
  return (
    <>
      {all.map((p) => (
        <Laid key={p.tab} page={p} />
      ))}
    </>
  );
}

// Under the menus, dialogs and Settings (z-40, z-50); hidden until the loop lays it over its slot.
const STYLE = { position: 'fixed', zIndex: 1, visibility: 'hidden', border: 0 } as const;

function Laid({ page: { tab, url, reload } }: { page: Page }) {
  const ref = (el: HTMLElement | null) => {
    if (!el) return;
    els.set(tab, el);
    return () => void (els.get(tab) === el && els.delete(tab));
  };
  return WEBVIEW ? (
    <webview ref={ref} src={url} partition="preview" style={STYLE} />
  ) : (
    <iframe key={reload} ref={ref} src={url} title={url} style={STYLE} className="bg-bg" />
  );
}
