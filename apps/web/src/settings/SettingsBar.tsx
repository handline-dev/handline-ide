// A settings detail page's navigation bar, at the top of the page under the tab bar: back to its list, and the page's name.
import { ChevronLeft } from 'lucide-react';
import type { ReactNode } from 'react';

/** A settings detail page's bar, as a phone's navigation bar: "‹ Devices" at the left goes back to the list, the page's icon and
 *  name in the middle. */
export function SettingsBar({ title, icon, back, backTo }: { title: ReactNode; icon?: ReactNode; back: () => void; backTo: string }) {
  return (
    <div className="flex w-full items-center gap-2 border-b border-border pb-2">
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
    </div>
  );
}
