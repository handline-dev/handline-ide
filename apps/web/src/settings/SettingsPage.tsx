// One frame for every Settings screen: the title at the top, one sentence under it, the screen's one main button at the right, then its cards.
import type { ReactNode } from 'react';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

export function SettingsPage({
  keep,
  bar,
  title,
  note,
  action,
  room = '',
  children,
}: {
  /** Space kept under the last row, for a bar floating over the page (selectionRoom). */
  room?: string;
  keep: string;
  /** A detail page's navigation bar, above all else. */
  bar?: ReactNode;
  /** None on a detail page, whose name the bar carries. */
  title?: string;
  note?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <ScrollFade keep={keep} className={`min-h-0 flex-1 ${room}`}>
      <Page>
        <div className="flex flex-col gap-4 pb-8 pt-3">
          {bar}
          <div className="flex items-end justify-between gap-4">
            <div>
              {title && <h1 className="text-xl font-semibold text-fg">{title}</h1>}
              {note && <p className="text-sm text-muted">{note}</p>}
            </div>
            {action}
          </div>
          {children}
        </div>
      </Page>
    </ScrollFade>
  );
}
