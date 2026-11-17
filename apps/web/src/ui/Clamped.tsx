import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';

/** Four lines, then more, which shows the rest in place; a shorter text has no more. */
export function Clamped({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const [whole, setWhole] = useState(false);
  const [long, setLong] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el) setLong(el.scrollHeight > el.clientHeight + 1);
  }, [children]);
  return (
    <div>
      <div ref={ref} className={whole ? '' : 'line-clamp-4'}>
        {children}
      </div>
      {long && !whole && (
        <button type="button" onClick={() => setWhole(true)} className="rounded-sm text-sm text-accent hover:bg-hover/30">
          more
        </button>
      )}
    </div>
  );
}
