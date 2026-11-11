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
export function Block({ title, note, children }: { title: string; note?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border p-4">
      <div>
        <h2 className="text-sm font-medium text-fg">{title}</h2>
        {note && <p className="text-xs text-muted">{note}</p>}
      </div>
      {children}
    </section>
  );
}

/** One setting: what it is and what it does at the left, its control at the right (under it when narrow). */
export function Setting({ title, note, children }: { title: string; note?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-border pt-3 first:border-t-0 first:pt-0">
      <div className="min-w-0 flex-1 basis-56">
        <div className="text-sm text-fg">{title}</div>
        {note && <p className="text-xs text-muted">{note}</p>}
      </div>
      {children}
    </div>
  );
}
