import type { ReactNode } from 'react';
import { useOpenable } from '#src/layout/openable.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';

/** A sidebar row: click shows it in the workspace, drag drops it on a tab bar or a pane's edge. */
export function Row({
  opens,
  title,
  icon,
  selected = false,
  trail,
  note,
  unread = 0,
  menu = true,
  onShow,
}: {
  opens: string;
  title: string;
  icon: ReactNode;
  selected?: boolean;
  /** Drawn at the right end, muted (a time). */
  trail?: ReactNode;
  /** A second line under the title, one line long (an unread message). */
  note?: string;
  /** How many messages the note stands for, shown beside it when more than none. */
  unread?: number;
  /** False when the row sits in a menu of its own (an agent's), which has the open items at its top. */
  menu?: boolean;
  onShow: () => void;
}) {
  const spring = useSpringOpen(onShow);
  return (
    <div
      {...spring.props}
      role="button"
      aria-current={selected || undefined}
      tabIndex={0}
      {...useOpenable(opens, { here: onShow, menu })}
      className="group flex cursor-default flex-col rounded-md px-2 py-1 text-base text-fg outline-none select-none focus-visible:bg-hover aria-[current]:bg-selected data-[spring]:bg-hover"
    >
      <span className="flex h-5 items-center gap-1.5 group-aria-[current]:font-medium">
        <span className="grid size-4 shrink-0 place-items-center text-muted">{icon}</span>
        <span className="flex-1 truncate">{title}</span>
        {trail && <span className="shrink-0 text-xs text-muted tabular-nums">{trail}</span>}
      </span>
      {/* Under the title, from the name's left edge. */}
      {note && (
        <span className="flex items-center gap-1.5 text-xs text-muted">
          <span className="w-4 shrink-0" />
          <span className="flex-1 truncate">{note}</span>
          {unread > 0 && <span className="shrink-0 rounded-full bg-accent px-1.5 font-medium text-accent-fg tabular-nums">{unread}</span>}
        </span>
      )}
    </div>
  );
}
