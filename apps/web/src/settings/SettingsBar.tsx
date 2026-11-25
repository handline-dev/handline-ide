// The Settings top bar: empty on a section's own page; a detail page (one device, one agent) puts its back arrow and title there.
import { ArrowLeft } from 'lucide-react';
import { createContext, useContext, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

const Slot = createContext<HTMLElement | null>(null);

/** The bar itself, 40px like the app's tab bar; the window drags by it except on its controls. */
export function SettingsBarFrame({ children }: { children: ReactNode }) {
  const [node, setNode] = useState<HTMLElement | null>(null);
  return (
    <>
      <div className="app-drag layer-drag flex h-10 shrink-0 items-center border-b border-border bg-surface px-3">
        <div ref={setNode} className="flex min-w-0 items-center" />
      </div>
      <Slot.Provider value={node}>{children}</Slot.Provider>
    </>
  );
}

/** Back arrow and title in the top bar, for a settings detail page. */
export function SettingsBar({ title, back, backTo }: { title: ReactNode; back: () => void; backTo: string }) {
  const node = useContext(Slot);
  if (!node) return null;
  return createPortal(
    <div className="flex min-w-0 items-center gap-1.5">
      <button
        type="button"
        aria-label={`Back to ${backTo}`}
        title={`Back to ${backTo}`}
        onClick={back}
        className="app-no-drag flex size-7 shrink-0 items-center justify-center rounded-md text-muted hover:bg-hover/30 hover:text-fg"
      >
        <ArrowLeft size={16} />
      </button>
      <span className="min-w-0 truncate text-sm font-medium text-fg">{title}</span>
    </div>,
    node,
  );
}
