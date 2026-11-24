import { ArrowDown } from 'lucide-react';
import { Dots } from '#src/chat/Dots.tsx';
import { NEAR } from '#src/chat/useChatScroll.ts';
import { useEffect, useState, type RefObject } from 'react';

/** The round button centred over a scroll area's bottom while the reader is above its end: the chat's working dots while the agent works, else an arrow. */
export function JumpDown({
  box,
  working = false,
  onDown,
}: {
  box: RefObject<HTMLElement | null>;
  working?: boolean;
  /** Down to the end; a plain glide when not given. */
  onDown?: () => void;
}) {
  const [away, setAway] = useState(false);
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const read = () => setAway(el.scrollHeight - el.scrollTop - el.clientHeight >= NEAR);
    read();
    el.addEventListener('scroll', read, { passive: true });
    const grow = new ResizeObserver(read);
    grow.observe(el);
    for (const c of el.children) grow.observe(c);
    return () => {
      el.removeEventListener('scroll', read);
      grow.disconnect();
    };
  }, [box]);
  if (!away) return null;
  return (
    <button
      type="button"
      aria-label={working ? 'Working, go to the latest' : 'Go to the latest'}
      title="Go to the latest"
      onClick={onDown ?? (() => box.current?.scrollTo({ top: box.current.scrollHeight, behavior: 'smooth' }))}
      className="absolute bottom-3 left-1/2 z-10 grid size-9 -translate-x-1/2 place-items-center rounded-full border border-border bg-surface text-fg hover:bg-hover"
    >
      {working ? <Dots /> : <ArrowDown size={16} strokeWidth={2.5} />}
    </button>
  );
}
