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

/** A titled block of a settings section. */
export function Block({ title, note, children }: { title: string; note?: ReactNode; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2 py-4">
      <div>
        <h3 className="text-sm font-medium text-fg">{title}</h3>
        {note && <p className="text-xs text-muted">{note}</p>}
      </div>
      {children}
    </section>
  );
}
