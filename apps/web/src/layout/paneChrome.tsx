// The parts of a pane's frame around its tabs: what its handle does by default, its split button, and the divider between two panes,
// which follows the pointer and comes to rest at a third, a half or two thirds.
import { ArrowDownToLine, ArrowLeftToLine, ArrowRightToLine, ArrowUpToLine, Columns2, LayoutGrid, X } from 'lucide-react';
import { useRef, useState, type ReactNode } from 'react';
import type { Dispatch, Grip } from '#src/Workspace.tsx';
import { snapRatio, stepStop, type Edge, type Split, type Target } from '#src/layout/tree.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';

/** The whole tab bar `pane` goes where its handle is let go: into another pane, or beside it. */
export const gripIn =
  (dispatch: Dispatch): Grip =>
  (pane) => ({
    onDrop: (drop) => {
      if (!drop || !('pane' in drop) || (drop.pane === pane.id && drop.zone === 'center')) return;
      const to: Target = drop.zone === 'center' ? { pane: drop.pane } : { pane: drop.pane, edge: drop.zone };
      dispatch({ type: 'move', from: pane.id, tab: pane.active, tabs: pane.tabs.map((t) => t.id), to });
    },
  });

const splits: { edge: Edge | 'four'; label: string; icon: ReactNode }[] = [
  { edge: 'right', label: 'Split right', icon: <ArrowRightToLine size={14} /> },
  { edge: 'left', label: 'Split left', icon: <ArrowLeftToLine size={14} /> },
  { edge: 'down', label: 'Split down', icon: <ArrowDownToLine size={14} /> },
  { edge: 'up', label: 'Split up', icon: <ArrowUpToLine size={14} /> },
  { edge: 'four', label: 'Split in four', icon: <LayoutGrid size={14} /> },
];

/** Click splits the pane right; right-click picks where, or closes the pane. */
export function SplitButton({ onSplit, onClose }: { onSplit: (edge: Edge | 'four') => void; onClose?: () => void }) {
  return (
    <ContextMenu
      items={
        <>
          {splits.map((s) => (
            <MenuItem key={s.edge} icon={s.icon} onClick={() => onSplit(s.edge)}>
              {s.label}
            </MenuItem>
          ))}
          {onClose && (
            <>
              <MenuSeparator />
              <MenuItem icon={<X size={14} />} onClick={onClose}>
                Close pane
              </MenuItem>
            </>
          )}
        </>
      }
    >
      <IconButton label="Split right (right-click for more)" onClick={() => onSplit('right')}>
        <Columns2 size={16} />
      </IconButton>
    </ContextMenu>
  );
}

export function SplitView({ node, dispatch, children }: { node: Split; dispatch: Dispatch; children: [ReactNode, ReactNode] }) {
  const box = useRef<HTMLDivElement>(null);
  const x = node.axis === 'x';
  const resize = (ratio: number) => dispatch({ type: 'resize', split: node.id, ratio });
  // It follows the pointer, and comes to rest at the nearest stop when let go.
  const [dragging, setDragging] = useState(false);
  const glide = dragging ? undefined : 'flex-grow 200ms ease-out';
  return (
    <div ref={box} className={`flex min-h-0 min-w-0 flex-1 ${x ? 'flex-row' : 'flex-col'}`}>
      <div className="flex min-h-0 min-w-0" style={{ flex: `${node.ratio} 1 0`, transition: glide }}>
        {children[0]}
      </div>
      <Splitter
        axis={node.axis}
        container={box}
        percent={node.ratio * 100}
        onMove={(o, size) => resize(o / size)}
        onStep={(d) => resize(stepStop(node.ratio, d))}
        onDragging={(on) => {
          setDragging(on);
          if (!on) resize(snapRatio(node.ratio));
        }}
        label={x ? 'Resize columns' : 'Resize rows'}
      />
      <div className="flex min-h-0 min-w-0" style={{ flex: `${1 - node.ratio} 1 0`, transition: glide }}>
        {children[1]}
      </div>
    </div>
  );
}
