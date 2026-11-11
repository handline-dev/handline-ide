// One frame for every Settings screen: the title at the top, one sentence under it, the screen's one main button at the right, then its cards.
import type { ReactNode } from 'react';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

export function SettingsPage({
  keep,
  title,
  note,
  action,
  children,
}: {
  keep: string;
  title: string;
  note: ReactNode;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <ScrollFade keep={keep} className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-4 pb-8 pt-3">
          <div className="flex items-end justify-between gap-4">
            <div>
              <h1 className="text-xl font-semibold text-fg">{title}</h1>
              <p className="text-sm text-muted">{note}</p>
            </div>
            {action}
          </div>
          {children}
        </div>
      </Page>
    </ScrollFade>
  );
}
