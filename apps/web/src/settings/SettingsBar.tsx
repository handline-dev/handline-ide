// The Settings top bar: empty on a section's own page; a detail page (one device, one agent) puts its back arrow and title there.
import { ChevronLeft } from 'lucide-react';
import { createContext, useContext, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

const Slot = createContext<HTMLElement | null>(null);

/** The bar itself, 40px like the app's tab bar; the window drags by it except on its controls. */
export function SettingsBarFrame({ children }: { children: ReactNode }) {
  const [node, setNode] = useState<HTMLElement | null>(null);
  return (
    <>
      <div className="app-drag layer-drag flex h-10 shrink-0 items-center border-b border-border bg-surface px-3">
        <div ref={setNode} className="flex min-w-0 flex-1 items-center" />
      </div>
      <Slot.Provider value={node}>{children}</Slot.Provider>
    </>
  );
}

/** A settings detail page's bar, as a phone's navigation bar: "‹ Devices" at the left goes back to the list, the page's icon and
 *  name in the middle. */
export function SettingsBar({ title, icon, back, backTo }: { title: ReactNode; icon?: ReactNode; back: () => void; backTo: string }) {
  const node = useContext(Slot);
  if (!node) return null;
  return createPortal(
    <div className="flex w-full items-center gap-2">
      <span className="flex min-w-0 flex-1 basis-0">
        <button
          type="button"
          onClick={back}
          className="app-no-drag flex min-w-0 items-center gap-0.5 rounded-md py-1 pr-2 text-sm text-accent hover:text-fg"
        >
          <ChevronLeft size={18} className="shrink-0" />
          <span className="truncate">{backTo}</span>
        </button>
      </span>
      <span className="flex min-w-0 shrink items-center gap-2 text-sm font-semibold text-fg">
        {icon}
        <span className="truncate">{title}</span>
      </span>
      <span className="flex-1 basis-0" />
    </div>,
    node,
  );
}
