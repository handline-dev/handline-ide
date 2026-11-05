import { howOf, type How } from '#src/layout/openTab.tsx';
import { Plus, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';

/** `closable: false` pins a tab: no close button, and onClose is never called for it. */
export type TabItem = { id: string; title: string; closable?: boolean; icon?: ReactNode };

export function Tab({
  tab,
  active,
  onSelect,
  onClose,
  onDragStart,
}: {
  tab: TabItem;
  active: boolean;
  onSelect: () => void;
  onClose: () => void;
  /** Makes the tab draggable (pinned tabs never are); fill the drag's dataTransfer here. */
  onDragStart?: (e: React.DragEvent) => void;
}) {
  const closable = tab.closable !== false;
  const draggable = closable && !!onDragStart;
  return (
    <div
      role="tab"
      draggable={draggable}
      onDragStart={draggable ? onDragStart : undefined}
      aria-selected={active}
      onClick={onSelect}
      className={`group flex h-7 max-w-48 min-w-20 cursor-default items-center gap-1 rounded-md text-sm ${
        closable ? 'pr-1.5 pl-3' : 'px-2.5'
      } ${active ? 'bg-hover text-fg' : 'text-muted hover:bg-hover hover:text-fg'}`}
    >
      {tab.icon && <span className="grid shrink-0 place-items-center text-muted">{tab.icon}</span>}
      <span className="flex-1 truncate">{tab.title}</span>
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
  newTabKinds = [],
  onAdd,
  onTabDragStart,
}: {
  tabs: TabItem[];
  active: string | null;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  /** What + can open: a menu of these; picking one calls onAdd with its kind. */
  newTabKinds?: { kind: string; label: string; icon?: ReactNode }[];
  onAdd?: (kind: string, how: How) => void;
  onTabDragStart?: (id: string, e: React.DragEvent) => void;
}) {
  return (
    <div role="tablist" className="flex h-10 items-center gap-1 border-b border-border bg-surface px-2">
      {tabs.map((t) => (
        <Tab
          key={t.id}
          tab={t}
          active={t.id === active}
          onSelect={() => onSelect(t.id)}
          onClose={() => t.closable !== false && onClose(t.id)}
          onDragStart={onTabDragStart && ((e) => onTabDragStart(t.id, e))}
        />
      ))}
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
