import type { MouseEvent, ReactNode } from 'react';

/** A muted heading over a group of menu items. */
export function MenuHeading({ children }: { children: ReactNode }) {
  return <div className="px-2 pt-1 pb-0.5 text-xs text-muted">{children}</div>;
}

/** A rule between groups of menu items. */
export function MenuRule() {
  return <div className="my-1 h-px bg-border" />;
}

/** A line of facts in a menu: a muted name, its value at the end. */
export function MenuFact({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-6 px-2 py-0.5 text-xs">
      <span className="text-muted">{name}</span>
      <span className="text-fg tabular-nums">{children}</span>
    </div>
  );
}

/** A quiet item of the line under the message box: its hover is faint, and it reads as text until the pointer comes. */
export const chip =
  '-mx-2 flex min-w-0 items-center gap-1 rounded-md px-2 py-1 outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent data-[popup-open]:text-fg';

/** An action row at the foot of a hover card, nearest the pointer. */
export function CardAction({ icon, onClick, children }: { icon: ReactNode; onClick: (e: MouseEvent) => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex h-7 w-full items-center gap-2 rounded-sm px-2 text-sm text-fg outline-none hover:bg-hover focus-visible:bg-hover"
    >
      <span className="text-muted">{icon}</span>
      {children}
    </button>
  );
}
