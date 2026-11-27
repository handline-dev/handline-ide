import type { ReactNode } from 'react';

/** A titled card of a settings section: its title (with `action` beside it), one muted sentence, then its rows. */
export function Block({
  title,
  icon,
  action,
  note,
  children,
}: {
  title: string;
  icon?: ReactNode;
  action?: ReactNode;
  note?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border p-4">
      <div>
        <h2 className="flex items-center gap-2 text-sm font-medium text-fg">
          {icon && <span className="text-muted">{icon}</span>}
          {title}
          {action}
        </h2>
        {note && <p className="text-xs text-muted">{note}</p>}
      </div>
      {children}
    </section>
  );
}

/** One setting: what it is and what it does at the left, its control at the right (under it when narrow), and `below` across the full width. */
export function Setting({
  title,
  icon,
  note,
  below,
  children,
}: {
  title: ReactNode;
  icon?: ReactNode;
  note?: ReactNode;
  below?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border pt-3 first:border-t-0 first:pt-0">
      <div className="min-w-0 flex-1 basis-56">
        <div className="flex items-center gap-2 text-sm text-fg">
          {icon && <span className="text-muted">{icon}</span>}
          {title}
        </div>
        {note && <p className="text-xs text-muted">{note}</p>}
      </div>
      {children}
      {below && <div className="flex basis-full flex-col gap-1">{below}</div>}
    </div>
  );
}
