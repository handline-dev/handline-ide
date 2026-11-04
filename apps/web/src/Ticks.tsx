import { Check } from 'lucide-react';

/** How far a message of mine got, as two checks: none lit while sending, one when sent, both when read. */
export function Ticks({ level }: { level: 0 | 1 | 2 }) {
  const word = ['Sending', 'Sent', 'Read'][level];
  const tone = (on: boolean) => (on ? 'text-success' : 'text-border');
  // The lit check is drawn in front, with a page-coloured border so the two do not run together.
  const [back, front] = level === 1 ? (['left-1.5', 'left-0'] as const) : (['left-0', 'left-1.5'] as const);
  return (
    <span aria-label={word} title={word} className="relative inline-flex h-3.5 w-5">
      <Check size={14} strokeWidth={3} className={`absolute ${back} ${tone(level === 2)}`} />
      <Check size={14} strokeWidth={7} className={`absolute ${front} text-bg`} />
      <Check size={14} strokeWidth={3} className={`absolute ${front} ${tone(level >= 1)}`} />
    </span>
  );
}
