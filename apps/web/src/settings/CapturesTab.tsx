// Settings > Captures: the windows agents captured with hl screen, newest first; a row with its picture opens it.
import { useRead, useReadScreenPicture, type ReadResult } from '@handline/app-data';
import { Camera, ImageOff } from 'lucide-react';
import { useEffect, useState } from 'react';
import { ago, useNow } from '#src/format/time.ts';
import { SettingsPage } from '#src/settings/SettingsPage.tsx';
import { Button } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ImageViewer } from '#src/viewer/ImageViewer.tsx';

type Row = NonNullable<ReadResult<'screen.history'>['data']>['captures'][number];

function useShown() {
  const read = useReadScreenPicture();
  const [shown, setShown] = useState<{ url: string; name: string } | null>(null);
  useEffect(() => () => void (shown && URL.revokeObjectURL(shown.url)), [shown]);
  const open = async (r: Row) => {
    try {
      const url = URL.createObjectURL(await read(r.id));
      setShown({ url, name: `${r.app} by ${r.agent}` });
    } catch {
      toast('Picture failed', { tone: 'warning' });
    }
  };
  return { shown, open, close: () => setShown(null) };
}

export function CapturesTab() {
  const history = useRead('screen.history', {});
  const now = useNow();
  const { shown, open, close } = useShown();
  const rows = history.data?.captures ?? [];
  return (
    <SettingsPage
      keep="settings-captures"
      title="Captures"
      action={
        <Button disabled={history.isFetching} onClick={() => void history.refetch()}>
          Refresh
        </Button>
      }
    >
      <div className="flex flex-col rounded-lg border border-border px-4">
        {history.isPending && <p className="py-3 text-sm text-muted">Looking…</p>}
        {history.isError && <p className="py-3 text-sm text-danger">Load failed</p>}
        {history.isSuccess && rows.length === 0 && <p className="py-3 text-sm text-muted">None yet</p>}
        {rows.map((r) => (
          <button
            key={r.id}
            type="button"
            data-capture={r.id}
            disabled={!r.has_picture}
            onClick={() => void open(r)}
            className="flex items-center gap-3 border-t border-border py-3 text-left first:border-t-0 enabled:hover:bg-hover/30 disabled:cursor-default"
          >
            {r.has_picture ? <Camera size={14} className="shrink-0 text-muted" /> : <ImageOff size={14} className="shrink-0 text-muted" />}
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm text-fg">
                {r.app || 'Unknown'}
                {r.title && <span className="text-muted"> · {r.title}</span>}
              </div>
              <div className="truncate text-xs text-muted">
                {r.agent}
                {r.task > 0 && ` · T${r.task}`}
                {!r.ok && ` · ${r.status}${r.reason ? `: ${r.reason}` : ''}`}
                {r.ok && !r.has_picture && ' · file gone'}
              </div>
            </div>
            <span className="shrink-0 text-xs tabular-nums text-muted">{ago(r.at, now)}</span>
          </button>
        ))}
      </div>
      {shown && <ImageViewer pictures={[{ url: shown.url, name: shown.name }]} onClose={close} />}
    </SettingsPage>
  );
}
