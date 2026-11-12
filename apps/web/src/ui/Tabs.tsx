import { howOf, type How } from '#src/layout/openTab.tsx';
import { Plus, X } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { clickPick, dragged, noPick, type Pick } from '#src/ui/tabPick.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
import { ContextMenu, Menu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';

/** `closable: false` pins a tab: no close button, and onClose is never called for it. `dot`: what marks it right of its label. */
export type TabItem = { id: string; title: string; closable?: boolean; icon?: ReactNode; dot?: ReactNode };

export function Tab({
  tab,
  active,
  picked = false,
  faded = false,
  onSelect,
  onPress,
  onClose,
  onDragStart,
  onDragEnd,
}: {
  tab: TabItem;
  active: boolean;
  /** One of several tabs picked together (Cmd/Ctrl+click, Shift+click). */
  picked?: boolean;
  /** Being dragged away with the others picked. */
  faded?: boolean;
  onSelect: () => void;
  /** A click, with its keys; without it a click selects. */
  onPress?: (e: React.MouseEvent) => void;
  onDragEnd?: () => void;
  onClose: () => void;
  /** Makes the tab draggable, a pinned one too; fill the drag's dataTransfer here. */
  onDragStart?: (e: React.DragEvent) => void;
}) {
  const closable = tab.closable !== false;
  const draggable = !!onDragStart;
  // Files held over a tab bring it up, so they can be dropped in it.
  const spring = useSpringOpen(onSelect);
  // The shown tab is always in view of a bar that scrolls.
  const el = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (active) el.current?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
  }, [active]);
  return (
    <div
      ref={el}
      {...spring.props}
      role="tab"
      draggable={draggable}
      onDragStart={draggable ? onDragStart : undefined}
      onDragEnd={onDragEnd}
      aria-selected={active}
      onClick={onPress ?? onSelect}
      className={`group flex h-7 select-none max-w-48 min-w-20 shrink-0 cursor-default items-center gap-1 rounded-md text-sm ${
        closable ? 'pr-1.5 pl-3' : 'px-2.5'
      } ${
        picked
          ? 'bg-accent/12 text-fg ring-1 ring-accent/45 ring-inset'
          : active
            ? 'bg-hover text-fg'
            : 'text-muted hover:bg-hover hover:text-fg'
      } ${faded ? 'opacity-35' : ''}`}
    >
      {tab.icon && <span className="grid shrink-0 place-items-center text-muted">{tab.icon}</span>}
      <span className="flex-1 truncate">{tab.title}</span>
      {tab.dot}
      {closable && (
        <span className={active ? '' : 'invisible group-hover:visible'}>
          <IconButton
            size="sm"
            label={`Close ${tab.title}`}
            onClick={(e) => {
              e.stopPropagation();
              onClose();
            }}
          >
            <X size={12} />
          </IconButton>
        </span>
      )}
    </div>
  );
}

export function TabBar({
  tabs,
  active,
  onSelect,
  onClose,
  onCloseTabs,
  newTabKinds = [],
  onAdd,
  onTabDragStart,
}: {
  tabs: TabItem[];
  active: string | null;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  /** Right-click on a tab: Close others, Close tabs to the right, Close all (pinned tabs stay). */
  onCloseTabs?: (ids: string[]) => void;
  /** What + can open: a menu of these; picking one calls onAdd with its kind. */
  newTabKinds?: { kind: string; label: string; icon?: ReactNode }[];
  onAdd?: (kind: string, how: How) => void;
  /** A drag from tab `id`; `ids` is what it carries: the picked tabs when `id` is one of them, else `id` alone. */
  onTabDragStart?: (id: string, ids: string[], e: React.DragEvent) => void;
}) {
  const [pick, setPick] = useState<Pick>(noPick);
  const strip = useRef<HTMLDivElement>(null);
  const fade = useScrollFade(strip); // the edges fade while more tabs are off that side
  const [dragging, setDragging] = useState(false);
  const ids = tabs.map((t) => t.id);
  const key = ids.join('|');
  // A move out of this bar unmounts the dragged tab, so its dragend may never come: a changed bar lets go of the drag.
  const [seen, setSeen] = useState(key);
  if (seen !== key) {
    setSeen(key);
    setDragging(false);
  }
  const shown = new Set(pick.picked.length > 1 ? pick.picked.filter((id) => ids.includes(id)) : []);
  // Escape lets go of the pick.
  useEffect(() => {
    if (!shown.size) return;
    const off = (e: KeyboardEvent) => e.key === 'Escape' && !e.defaultPrevented && setPick(noPick);
    window.addEventListener('keydown', off);
    return () => window.removeEventListener('keydown', off);
  }, [shown.size]);
  const click = (id: string, e: React.MouseEvent) => {
    const next = clickPick(pick, ids, active, id, { add: e.metaKey || e.ctrlKey, range: e.shiftKey });
    setPick(next);
    if (next.show) onSelect(id);
  };
  return (
    // Tabs keep their width; the strip scrolls sideways, by wheel too; + stays at the end.
    <div className="flex h-10 items-center gap-1 border-b border-border bg-surface px-2">
      <div
        ref={strip}
        style={fade}
        role="tablist"
        aria-multiselectable
        onWheel={(e) => {
          if (Math.abs(e.deltaY) > Math.abs(e.deltaX)) e.currentTarget.scrollLeft += e.deltaY;
        }}
        className="flex min-w-0 items-center gap-1 overflow-x-auto"
      >
        {tabs.map((t, i) => {
          const tab = (
            <Tab
              key={t.id}
              tab={t}
              active={t.id === active}
              picked={shown.has(t.id)}
              faded={dragging && shown.has(t.id)}
              onSelect={() => onSelect(t.id)}
              onPress={(e) => click(t.id, e)}
              onClose={() => t.closable !== false && onClose(t.id)}
              onDragStart={
                onTabDragStart &&
                ((e) => {
                  const carried = dragged({ ...pick, picked: [...shown] }, t.id);
                  if (carried.length > 1) {
                    setDragging(true);
                    dragLabel(e, `${carried.length} tabs`);
                  }
                  onTabDragStart(t.id, carried, e);
                })
              }
              onDragEnd={() => {
                if (dragging) setPick(noPick);
                setDragging(false);
              }}
            />
          );
          if (!onCloseTabs) return tab;
          const closable = (l: TabItem[]) => l.filter((x) => x.closable !== false).map((x) => x.id);
          const others = closable(tabs.filter((x) => x.id !== t.id));
          const right = closable(tabs.slice(i + 1));
          const picked = shown.has(t.id) ? closable(tabs.filter((x) => shown.has(x.id))) : [];
          return (
            <ContextMenu
              key={t.id}
              items={
                <>
                  {picked.length > 1 ? (
                    <MenuItem
                      onClick={() => {
                        setPick(noPick);
                        onCloseTabs(picked);
                      }}
                    >
                      Close {picked.length} picked tabs
                    </MenuItem>
                  ) : (
                    <MenuItem disabled={t.closable === false} onClick={() => onClose(t.id)}>
                      Close
                    </MenuItem>
                  )}
                  <MenuItem disabled={!others.length} onClick={() => onCloseTabs(others)}>
                    Close others
                  </MenuItem>
                  <MenuItem disabled={!right.length} onClick={() => onCloseTabs(right)}>
                    Close tabs to the right
                  </MenuItem>
                  <MenuSeparator />
                  <MenuItem disabled={!closable(tabs).length} onClick={() => onCloseTabs(closable(tabs))}>
                    Close all
                  </MenuItem>
                </>
              }
            >
              {tab}
            </ContextMenu>
          );
        })}
      </div>
      {onAdd && newTabKinds.length > 0 && (
        <Menu
          trigger={
            <IconButton label="New tab">
              <Plus size={16} />
            </IconButton>
          }
        >
          {newTabKinds.map((k) => (
            <MenuItem key={k.kind} icon={k.icon} onClick={(e) => onAdd(k.kind, howOf(e))}>
              {k.label}
            </MenuItem>
          ))}
        </Menu>
      )}
    </div>
  );
}

/** The drag's picture: a chip with `text`, not the one tab under the pointer. */
function dragLabel(e: React.DragEvent, text: string) {
  const chip = document.createElement('div');
  chip.textContent = text;
  chip.className = 'fixed -top-20 left-0 rounded-md bg-accent px-2.5 py-1 text-xs text-accent-fg shadow-md';
  document.body.append(chip);
  e.dataTransfer.setDragImage(chip, -8, -8);
  requestAnimationFrame(() => chip.remove());
}
