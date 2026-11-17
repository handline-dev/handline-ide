import { useContext, type DragEvent, type ReactNode } from 'react';
import { mergeHandlers, useDropTarget } from '#src/ui/dropTarget.tsx';
import { TabsTo } from '#src/sidebar/tabDrop.ts';
import { isTabMove, readTabDrag } from '#src/ui/dnd.tsx';
import { carriesFiles } from '#src/ui/fileDrop.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';

/** A sidebar row: click shows it in the workspace, drag drops it on a tab bar or a pane's edge. With `space`, a tab dragged out of a
 *  pane and dropped on the row moves to that workspace. */
export function Row({
  opens,
  title,
  icon,
  selected = false,
  trail,
  note,
  unread = 0,
  menu = true,
  space,
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
  /** The workspace the row brings up, which takes tabs dropped on it. */
  space?: string;
  onShow: () => void;
}) {
  const tabsTo = useContext(TabsTo);
  const takes = (e: DragEvent) => !!space && !!tabsTo && isTabMove(e);
  // Held there, dragged files or a tab open the row's workspace, to be dropped where in it they go.
  const spring = useSpringOpen(onShow, (e) => carriesFiles(e) || takes(e));
  const drop = useDropTarget({
    accepts: takes,
    dropEffect: 'move',
    onDrop: (e) => {
      const drag = readTabDrag(e);
      if (drag && 'move' in drag) tabsTo!(drag.move, space!);
    },
  });
  const { onDragEnter, onDragLeave, onDrop, ...look } = spring.props;
  return (
    <div
      {...look}
      role="button"
      aria-current={selected || undefined}
      tabIndex={0}
      {...useOpenable(opens, { here: onShow, menu })}
      data-over={drop.over || undefined}
      {...mergeHandlers({ onDragEnter, onDragLeave, onDrop }, drop.handlers)}
      className="group flex cursor-default flex-col rounded-md px-2 py-1 text-base text-fg outline-none select-none focus-visible:bg-hover aria-[current]:bg-selected data-[over]:bg-hover data-[over]:ring-2 data-[over]:ring-accent data-[spring]:bg-hover"
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
