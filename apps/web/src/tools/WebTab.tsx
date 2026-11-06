import { ArrowLeft, ArrowRight, ExternalLink, RotateCw } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { usePersisted } from '#src/persist/kv.ts';
import { shell } from '#src/shell.ts';
import { ServerPicker } from '#src/tools/ServerPicker.tsx';
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

/** The shell has one native view per window: the first Web tab on screen takes it, any other shows an iframe. */
let owner: object | null = null;

/** A web page beside the chat, its address kept per tab: the shell's native view (every site shows), or an iframe in a browser. */
export function WebTab({ tab, agent, start = '' }: { tab: string; agent: string | null; start?: string }) {
  const [url, setUrl] = usePersisted(`tab/${tab}/url`, start);
  const [typed, setTyped] = useState(url);
  const [reload, setReload] = useState(0);
  const frame = useRef<HTMLIFrameElement>(null);
  const [me] = useState(() => (!owner && shell()?.preview ? (owner = {}) : null));
  useEffect(() => {
    if (!me) return;
    owner = me;
    return () => {
      if (owner === me) owner = null;
    };
  }, [me]);
  const native = me !== null;
  const open = (a: string) => {
    setUrl(a);
    setTyped(a);
    setReload((n) => n + 1);
  };
  const nav = (go: 'back' | 'forward' | 'reload') => {
    if (native) return void shell()?.previewNav?.(go);
    if (go === 'reload') return setReload((n) => n + 1);
    try {
      frame.current?.contentWindow?.history.go(go === 'back' ? -1 : 1); // a same-origin iframe only
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
          if (a) open(a);
        }}
      >
        <IconButton label="Back" onClick={() => nav('back')} disabled={!url}>
          <ArrowLeft size={14} />
        </IconButton>
        <IconButton label="Forward" onClick={() => nav('forward')} disabled={!url}>
          <ArrowRight size={14} />
        </IconButton>
        <IconButton label="Reload" onClick={() => nav('reload')} disabled={!url}>
          <RotateCw size={14} />
        </IconButton>
        {agent && <ServerPicker agent={agent} current={url} onPick={open} />}
        <Input
          autoFocus={!url}
          className="h-7 flex-1"
          placeholder="localhost:5173 or an address"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onFocus={(e) => e.target.select()}
        />
        {url && (
          <a
            href={url}
            target="_blank"
            rel="noreferrer"
            aria-label="Open in browser"
            title="Open in browser"
            className="grid size-7 place-items-center rounded-md text-muted hover:bg-hover/30"
          >
            <ExternalLink size={14} />
          </a>
        )}
      </form>
      {url && native ? (
        <NativeView url={url} />
      ) : url ? (
        <iframe key={`${url}#${reload}`} ref={frame} src={url} title={url} className="min-h-0 flex-1 border-0 bg-bg" />
      ) : (
        <p className="m-auto text-sm text-muted">Type an address to open a page.</p>
      )}
    </div>
  );
}

/** A menu, a dialog or a tab being dragged must reach what lies under the native view, which covers it: the view hides meanwhile. */
const covered = (dragging: boolean): boolean => dragging || !!document.querySelector('dialog[open], [aria-modal="true"], [role="menu"]');

/** The shell's view kept over this box in screen pixels, checked each frame and sent only on a change; hidden at zero size, so the
 * page keeps its state, and closed with the box. */
function NativeView({ url }: { url: string }) {
  const box = useRef<HTMLDivElement>(null);
  // The address is read each frame, so a new one loads in the same view and Back can return to the old one.
  const at = useRef(url);
  useEffect(() => {
    at.current = url;
  }, [url]);
  useEffect(() => {
    const s = shell();
    const el = box.current;
    if (!el || !s?.preview) return;
    let dragging = false;
    const drag = (on: boolean) => () => (dragging = on);
    const ons = [
      ['dragstart', drag(true)],
      ['dragend', drag(false)],
      ['drop', drag(false)],
    ] as const;
    for (const [e, f] of ons) addEventListener(e, f, true);
    let last = '';
    let frame = 0;
    const place = () => {
      const r = el.getBoundingClientRect();
      const z = s.zoom?.() || 1;
      const off = covered(dragging) || r.width < 1 || r.height < 1;
      const now = { url: at.current, x: r.left * z, y: r.top * z, width: off ? 0 : r.width * z, height: off ? 0 : r.height * z };
      const key = JSON.stringify(now);
      if (key !== last) {
        last = key;
        void s.preview!(now);
      }
      frame = requestAnimationFrame(place);
    };
    place();
    return () => {
      cancelAnimationFrame(frame);
      for (const [e, f] of ons) removeEventListener(e, f, true);
      void s.preview!(null);
    };
  }, []);
  return <div ref={box} className="min-h-0 flex-1 bg-bg" />;
}
