import { ArrowLeft, ArrowRight, RotateCw } from 'lucide-react';
import { useRef, useState } from 'react';
import { usePersisted } from '#src/persist/kv.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Input } from '#src/ui/Field.tsx';

/** What was typed as an address: `localhost:5173` is a page too. Null when it cannot be one. */
export function addressOf(typed: string): string | null {
  const t = typed.trim();
  if (!t) return null;
  const url = /^https?:\/\//i.test(t) ? t : `http://${t}`;
  try {
    return new URL(url).host ? url : null;
  } catch {
    return null;
  }
}

/** A web page beside the chat, its address kept per tab. For now an iframe: a site that refuses to be framed shows blank, and the
 * desktop shell's native view (layered, so menus draw over it) comes later. */
export function WebTab({ tab, start = '' }: { tab: string; start?: string }) {
  const [url, setUrl] = usePersisted(`tab/${tab}/url`, start);
  const [typed, setTyped] = useState(url);
  const [reload, setReload] = useState(0);
  const frame = useRef<HTMLIFrameElement>(null);
  // Back and forward reach into the page only when it is ours to read (same origin); elsewhere they do nothing.
  const step = (n: number) => {
    try {
      frame.current?.contentWindow?.history.go(n);
    } catch {
      /* another origin */
    }
  };
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <form
        className="flex h-9 shrink-0 items-center gap-1 border-b border-border px-2"
        onSubmit={(e) => {
          e.preventDefault();
          const a = addressOf(typed);
          if (!a) return;
          setUrl(a);
          setTyped(a);
          setReload((n) => n + 1);
        }}
      >
        <IconButton label="Back" onClick={() => step(-1)} disabled={!url}>
          <ArrowLeft size={14} />
        </IconButton>
        <IconButton label="Forward" onClick={() => step(1)} disabled={!url}>
          <ArrowRight size={14} />
        </IconButton>
        <IconButton label="Reload" onClick={() => setReload((n) => n + 1)} disabled={!url}>
          <RotateCw size={14} />
        </IconButton>
        <Input
          autoFocus={!url}
          className="h-7 flex-1"
          placeholder="localhost:5173 or an address"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onFocus={(e) => e.target.select()}
        />
      </form>
      {url ? (
        <iframe key={`${url}#${reload}`} ref={frame} src={url} title={url} className="min-h-0 flex-1 border-0 bg-bg" />
      ) : (
        <p className="m-auto text-sm text-muted">Type an address to open a page.</p>
      )}
    </div>
  );
}
