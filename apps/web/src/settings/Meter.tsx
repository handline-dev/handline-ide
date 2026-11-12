import type { ReactNode } from 'react';

const TONE = { ok: 'bg-accent', warn: 'bg-warning', bad: 'bg-danger' } as const;
export type Tone = keyof typeof TONE;

/** Calm under 60%, amber to 85%, red above (red for what is low, whatever the share). */
export const toneOf = (pct: number, low = false): Tone => (low || pct >= 85 ? 'bad' : pct >= 60 ? 'warn' : 'ok');

export function Bar({ pct, tone }: { pct: number; tone: Tone }) {
  return (
    <span className="block h-1.5 min-w-16 flex-1 overflow-hidden rounded-full bg-surface" aria-hidden>
      <span className={`block h-full ${TONE[tone]}`} style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
    </span>
  );
}

/** A titled card of a settings section: its title, one muted sentence, then its rows. */
export function Block({ title, icon, note, children }: { title: string; icon?: ReactNode; note?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border p-4">
      <div>
        <h2 className="flex items-center gap-2 text-sm font-medium text-fg">
          {icon && <span className="text-muted">{icon}</span>}
          {title}
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

/** One line of a list under a setting: an icon and a name at the left, what it is doing at the right. */
export function LineRow({ icon, who, what, title }: { icon: ReactNode; who: string; what: ReactNode; title?: string }) {
  return (
    <div className="flex items-center gap-2 text-xs text-muted" title={title}>
      {icon}
      <span className="min-w-0 flex-1 truncate text-fg">{who}</span>
      <span className="shrink-0">{what}</span>
    </div>
  );
}
