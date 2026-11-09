// The one way anything opens a tab: a click opens it here; Cmd+click (Ctrl+click off the Mac), a middle click or Cmd+Enter beside;
// a drag onto a tab bar, a pane or a pane's edge; a right-click offers Open, Open to the side, Open below and a copy (openItems).
// Every element that opens a tab takes its props from useOpenable, so a new one gets all of it.
import { Menu as M } from '@base-ui/react/menu';
import { Copy, ExternalLink, PanelBottom, PanelRight, SquareArrowOutUpRight } from 'lucide-react';
import { useEffect, useState, type DragEvent, type KeyboardEvent, type MouseEvent, type ReactNode } from 'react';
import { howOf, useOpenTab, type How } from '#src/layout/openTab.tsx';
import { openItems, type OpenItem } from '#src/layout/openItems.ts';
import { copy } from '#src/ui/copy.ts';
import { startTabDrag } from '#src/ui/dnd.tsx';
import { MenuItem } from '#src/ui/Menu.tsx';
import { floatingSurface } from '#src/ui/Popover.tsx';

type Options = {
  /** Runs before an open by click, key or menu; not on a drag, which may come to nothing. */
  onOpen?: (how: How) => void;
  /** Says what the tab shows once open, a file or an account; runs before every open, a drag's as it starts. */
  focus?: () => void;
  /** In place of a plain open: the sidebar shows an agent's workspace; a row that opens no tab opens what it is about. */
  here?: () => void;
  /** False for an element with its own right-click menu, which puts <OpenItems> at its top instead: never two menus. */
  menu?: boolean;
};

/** Opens `opens` the app's way, with the options' extras. */
function useGo(opens: string | null, { onOpen, focus, here }: Options) {
  const openTab = useOpenTab();
  return (how: How) => {
    focus?.();
    onOpen?.(how);
    if (here && (how === 'here' || !opens)) here();
    else if (opens) openTab(opens, how);
  };
}

/** The props that make an element open `opens` (null: it opens no tab, only `here`). Plain Enter on a button or a link with an href
 * is left to its own click, so it does not open twice. */
export function useOpenable(opens: string | null, options: Options = {}) {
  const go = useGo(opens, options);
  return {
    onClick: (e: MouseEvent) => go(howOf(e)),
    onAuxClick: (e: MouseEvent) => {
      if (e.button !== 1) return;
      e.preventDefault();
      go('side');
    },
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key !== 'Enter') return;
      const side = e.metaKey || e.ctrlKey;
      if (!side && clicksOnEnter(e.currentTarget)) return;
      e.preventDefault();
      go(side ? 'side' : 'here');
    },
    ...(opens && {
      draggable: true,
      onDragStart: (e: DragEvent) => {
        options.focus?.();
        startTabDrag(e, { open: { opens, title: '' } });
      },
    }),
    ...(opens &&
      options.menu !== false && {
        onContextMenu: (e: MouseEvent) => {
          if (!showMenu) return; // no host (the gallery): the browser's own menu
          e.preventDefault();
          e.stopPropagation();
          showMenu({ x: e.clientX, y: e.clientY, items: openItems(opens), run: (item) => runItem(item, go) });
        },
      }),
  };
}

const clicksOnEnter = (el: Element) => el instanceof HTMLButtonElement || (el instanceof HTMLAnchorElement && !!el.href);

function runItem(item: OpenItem, go: (how: How) => void) {
  if (item.id === 'copy') void copy(item.text, item.name, { say: true });
  else if (item.id === 'browser') window.open(item.url, '_blank', 'noopener,noreferrer');
  else go(item.id);
}

const ICON: Record<OpenItem['id'], ReactNode> = {
  here: <SquareArrowOutUpRight size={14} />,
  side: <PanelRight size={14} />,
  below: <PanelBottom size={14} />,
  copy: <Copy size={14} />,
  browser: <ExternalLink size={14} />,
};

function Items({ items, run }: { items: OpenItem[]; run: (item: OpenItem) => void }) {
  return items.map((item) => (
    <MenuItem key={`${item.id}:${item.label}`} icon={ICON[item.id]} onClick={() => run(item)}>
      {item.label}
    </MenuItem>
  ));
}

/** The open items at the top of an element's own right-click menu (a sidebar agent, a Requests row). */
export function OpenItems({ opens, ...options }: { opens: string } & Options) {
  const go = useGo(opens, options);
  return <Items items={openItems(opens)} run={(item) => runItem(item, go)} />;
}

type Shown = { x: number; y: number; items: OpenItem[]; run: (item: OpenItem) => void };
let showMenu: ((s: Shown) => void) | null = null;

/** The right-click menu of every openable, mounted once at the app's root and opened at the pointer. */
export function OpenMenuHost() {
  const [open, setOpen] = useState(false);
  // Kept after closing, so the menu does not empty while it fades out.
  const [shown, setShown] = useState<Shown | null>(null);
  useEffect(() => {
    showMenu = (s) => {
      setShown(s);
      setOpen(true);
    };
    return () => {
      showMenu = null;
    };
  }, []);
  const anchor = shown && { getBoundingClientRect: () => DOMRect.fromRect({ x: shown.x, y: shown.y, width: 0, height: 0 }) };
  return (
    <M.Root open={open} onOpenChange={setOpen}>
      <M.Portal>
        {/* Offset from the pointer, so the release of a right-click does not land on Open. */}
        <M.Positioner anchor={anchor} side="bottom" align="start" sideOffset={2} alignOffset={2} collisionPadding={8} className="z-50">
          <M.Popup className={`${floatingSurface} min-w-40 p-1`}>{shown && <Items items={shown.items} run={shown.run} />}</M.Popup>
        </M.Positioner>
      </M.Portal>
    </M.Root>
  );
}
