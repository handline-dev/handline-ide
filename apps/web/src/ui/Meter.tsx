// How full something is, as a thin bar, and the one rule for when a share reads as a warning.

const TONE = { ok: 'bg-accent', warn: 'bg-warning', bad: 'bg-danger' } as const;
export type Tone = keyof typeof TONE;
export const toneFill = (tone: Tone) => TONE[tone];

/** Calm under 60%, amber to 85%, red above (red for what is low, whatever the share). */
export const toneOf = (pct: number, low = false): Tone => (low || pct >= 85 ? 'bad' : pct >= 60 ? 'warn' : 'ok');

/** `fill`, a colour class, stands in for the tone where the colour tells things apart rather than how full. */
export function Bar({ pct, tone = 'ok', fill }: { pct: number; tone?: Tone; fill?: string }) {
  return (
    <span className="block h-1.5 min-w-16 flex-1 overflow-hidden rounded-full bg-surface" aria-hidden>
      <span className={`block h-full ${fill ?? TONE[tone]}`} style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
    </span>
  );
}
