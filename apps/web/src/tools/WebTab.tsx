import { ArrowLeft, ArrowRight, Camera, ExternalLink, RotateCw } from 'lucide-react';
import { toChat } from '#src/chat/toChat.ts';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ImageViewer } from '#src/viewer/ImageViewer.tsx';
import { useEffect, useState } from 'react';
import { usePersisted } from '#src/persist/kv.ts';
import { shell } from '#src/shell.ts';
import { ServerPicker } from '#src/tools/ServerPicker.tsx';
import { WebSlot, webPage } from '#src/tools/webPages.tsx';
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

/** A web page beside the chat, its address kept per tab: a <webview> in the shell (every site shows), an iframe in a browser. */
export function WebTab({ tab, agent, start = '' }: { tab: string; agent: string | null; start?: string }) {
  const [url, setUrl] = usePersisted(`tab/${tab}/url`, start);
  const [typed, setTyped] = useState(url);
  const page = webPage(tab);
  const canShoot = !!shell()?.capturePreview;
  const openTab = useOpenTab();
  // The page as it is on screen, to mark up and send to the agent's chat (the shell's webview only: an iframe cannot be captured).
  const [shot, setShot] = useState<string | null>(null);
  const capture = async () => {
    const id = page.id();
    const r = id === null ? null : await shell()?.capturePreview?.(id);
    if (!r?.ok) return toast(`No screenshot: ${r?.message ?? 'the page is not showing'}`, { tone: 'warning' });
    setShot(URL.createObjectURL(new Blob([Uint8Array.from(atob(r.png), (c) => c.charCodeAt(0))], { type: 'image/png' })));
  };
  useEffect(() => () => void (shot && URL.revokeObjectURL(shot)), [shot]);
  const closeShot = () => setShot(null);
  const open = (a: string) => {
    if (a === url) page.reload();
    setUrl(a);
    setTyped(a);
  };
  const nav = (go: 'back' | 'forward' | 'reload') => (go === 'reload' ? page.reload() : page.go(go));
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
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
        {url && canShoot && agent && (
          <IconButton label="Screenshot to mark up and send to the chat" onClick={() => void capture()}>
            <Camera size={14} />
          </IconButton>
        )}
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
      {url ? <WebSlot tab={tab} url={url} /> : <p className="m-auto text-sm text-muted">Type an address to open a page.</p>}
      {shot && agent && (
        <ImageViewer
          pictures={[{ url: shot, name: 'page.png' }]}
          onClose={closeShot}
          doneLabel="Send to chat"
          sendBare
          onMarked={(marked) => {
            // The chat is brought up first, so a chat it mounts is the one that takes the picture.
            openTab('chat', 'here');
            setTimeout(() => toChat(agent, marked));
          }}
        />
      )}
    </div>
  );
}
