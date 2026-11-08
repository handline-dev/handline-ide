import type { ReactNode } from 'react';
import { labelOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { startTabDrag } from '#src/ui/dnd.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';

/** A sidebar row: click shows it in the workspace, drag drops it on a tab bar or a pane's edge. */
export function Row({
  opens,
  title,
  icon,
  lead,
  selected = false,
  trail,
  note,
  context,
  unread = 0,
  onShow,
}: {
  opens: string;
  title: string;
  icon: ReactNode;
  lead?: ReactNode;
  selected?: boolean;
  /** Drawn at the right end, muted (a time). */
  trail?: ReactNode;
  /** A second line under the title, one line long (an unread message). */
  note?: string;
  /** Muted before the title: the team a pinned sub-agent belongs to. */
  context?: string;
  /** How many messages the note stands for, shown beside it when more than none. */
  unread?: number;
  onShow: () => void;
}) {
  const spring = useSpringOpen(onShow);
  const openTab = useOpenTab();
  return (
    <div
      {...spring.handlers}
      data-spring={spring.hovering || undefined}
      role="button"
      aria-current={selected || undefined}
      tabIndex={0}
      draggable
      onDragStart={(e) => startTabDrag(e, { open: { opens, title: labelOf(opens) } })}
      {...openProps((how) => (how === 'here' ? onShow() : openTab(opens, 'side')))}
      onKeyDown={(e) => {
        if (e.key !== 'Enter') return;
        e.preventDefault();
        if (e.metaKey || e.ctrlKey) openTab(opens, 'side');
        else onShow();
      }}
      className="group flex cursor-default flex-col rounded-md px-2 py-1 text-base text-fg outline-none select-none focus-visible:bg-hover aria-[current]:bg-selected data-[spring]:bg-hover"
    >
      <span className="flex h-5 items-center gap-1.5 group-aria-[current]:font-medium">
        {lead}
        <span className="grid size-4 shrink-0 place-items-center text-muted">{icon}</span>
        <span className="flex-1 truncate">
          {context && <span className="text-muted">{context} / </span>}
          {title}
        </span>
        {trail && <span className="shrink-0 text-xs text-muted tabular-nums">{trail}</span>}
      </span>
      {/* Under the title, from the name's left edge: blanks as wide as the lead and the icon. */}
      {note && (
        <span className="flex items-center gap-1.5 text-xs text-muted">
          {lead && <span className="w-3 shrink-0" />}
          <span className="w-4 shrink-0" />
          <span className="flex-1 truncate">{note}</span>
          {unread > 0 && <span className="shrink-0 rounded-full bg-accent px-1.5 font-medium text-accent-fg tabular-nums">{unread}</span>}
        </span>
      )}
    </div>
  );
}
