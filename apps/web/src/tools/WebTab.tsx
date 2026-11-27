import { ArrowLeft, ArrowRight, Camera, ExternalLink, RotateCw, SquareDashedMousePointer } from 'lucide-react';
import { LAST_CHAT, toChat } from '#src/chat/toChat.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { ImageViewer } from '#src/viewer/ImageViewer.tsx';
import { useState } from 'react';
import { read, usePersisted } from '#src/persist/kv.ts';
import { toast } from '#src/ui/toast.tsx';
import { useNameOf } from '@handline/app-data';
import { shell } from '#src/shell.ts';
import { usePageShot } from '#src/tools/pageShot.ts';
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
export function WebTab({ tab, agent: shown, start = '' }: { tab: string; agent: string | null; start?: string }) {
  // A Web tab names no agent: it sends to the one the focused tab shows, else the one it had last, else the last chat opened.
  const [last, setLast] = useState(shown);
  if (shown && shown !== last) setLast(shown);
  const agent = shown ?? last ?? read<string | null>(LAST_CHAT, null);
  const nameOf = useNameOf();
  const to = agent ? nameOf(agent) : 'an agent';
  const needAgent = (go: () => void) => () => (agent ? go() : toast('Open an agent first', { tone: 'warning' }));
  const [url, setUrl] = usePersisted(`tab/${tab}/url`, start);
  const [typed, setTyped] = useState(url);
  const page = webPage(tab);
  const canShoot = !!shell()?.capturePreview;
  const canPick = !!shell()?.pickElement;
  // eslint-disable-next-line no-restricted-syntax -- sending a marked picture brings the chat up; no link is clicked
  const openTab = useOpenTab();
  // The page as it is on screen, or one element picked in it, to mark up and send to the agent's chat (the shell's webview only).
  const { shot, picking, capture, pick, close: closeShot } = usePageShot(page);
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
        {url && canShoot && (
          <IconButton label={`Screenshot to send to ${to}`} onClick={needAgent(() => void capture())}>
            <Camera size={14} />
          </IconButton>
        )}
        {url && canPick && (
          <IconButton
            label={picking ? 'Click an element in the page, or stop picking (Esc)' : `Pick an element to send to ${to}`}
            aria-pressed={picking}
            className="aria-pressed:bg-selected aria-pressed:text-accent"
            onClick={needAgent(() => void pick())}
          >
            <SquareDashedMousePointer size={14} />
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
          pictures={[{ url: shot.url, name: 'page.png' }]}
          seed={shot.seed && [shot.seed]}
          onClose={closeShot}
          doneLabel="Send to chat"
          sendBare
          pen
          canSend
          onMarked={(marked, send) => {
            // The chat is brought up first, so a chat it mounts is the one that takes the picture.
            openTab(chatOf(agent), 'here');
            if (!shown) toast(`Sent to ${nameOf(agent)}`);
            // A picked element's details ride on its picture.
            const out = shot.note ? marked.map((m) => ({ ...m, note: shot.note })) : marked;
            setTimeout(() => toChat(agent, out, send));
          }}
        />
      )}
    </div>
  );
}
