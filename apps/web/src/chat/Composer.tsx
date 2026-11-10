import { useSuggest, type Command } from '#src/chat/Suggest.tsx';
import { DropHint, DropOver } from '#src/ui/fileDrop.tsx';
import { ArrowUp, CornerDownLeft, Paperclip, Square, SquareTerminal } from 'lucide-react';
import { useContext, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { ImageViewer, type MarkedPicture } from '#src/viewer/ImageViewer.tsx';
import { FolderChip, UploadChip, type Attachments } from '#src/chat/attachments.tsx';
import { readDropped } from '#src/ui/fileDrop.tsx';

/** The box a message is written in: it grows with the text up to a cap, then scrolls; Enter sends, Shift+Enter breaks a line.
 * Under the text, a row: attach and `leading` on the left (the model), `trailing` and send on the right (the context). While the
 * agent works and nothing is written, send is Stop; Esc in the box stops it too, as in a terminal agent. A `!` first is bash mode:
 * the box runs a command in the agent's terminal, so it says so (an orange border, a Bash chip in the `!`'s place, mono text);
 * Esc, or Backspace with nothing written, goes back to a message. */
export function Composer({
  agent,
  draft,
  setDraft,
  attach,
  onSend,
  leading,
  trailing,
  running = false,
  onStop,
  locked,
  onMarked,
  above,
  placeholder,
  commands = [],
  sendLabel,
  onEmpty,
  focusOn,
}: {
  agent: string;
  draft: string;
  setDraft: (v: string) => void;
  attach: Attachments;
  /** `task`: the message opens a task for the agent, its first line the title. */
  onSend: (body: string, attachments: string[], task?: { title: string }) => void;
  leading?: ReactNode;
  trailing?: ReactNode;
  running?: boolean;
  onStop?: () => void;
  /** Why the box takes no message now (a compaction under way), shown in its place; absent when it takes one. */
  locked?: string;
  /** Takes the box's pictures marked up in the viewer; the marked copy replaces the picture. */
  onMarked?: (marked: MarkedPicture[]) => void;
  /** A row at the top of the box (what the message replies to); the box takes focus when it comes. */
  above?: ReactNode;
  /** The box's hint when empty; `Message <agent>` by default. */
  placeholder?: string;
  /** What `/` offers at the start of the line; `@` offers the agents always. */
  commands?: Command[];
  /** A word on the send button beside its arrow (Answer while the box answers the founder's turn). */
  sendLabel?: string;
  /** Send with nothing written or going up runs this, with its label on the button, instead of staying disabled. */
  onEmpty?: { label: string; run: () => void };
  /** The box takes the focus each time this changes to something: a reply picked. */
  focusOn?: unknown;
}) {
  const dropOver = useContext(DropOver);
  const [viewing, setViewing] = useState<number | null>(null);
  const shown = attach.uploads.filter((d) => d.preview);
  const picker = useRef<HTMLInputElement>(null);
  const box = useRef<HTMLTextAreaElement>(null);
  // Height follows the text: read it at no height, so it can shrink as well as grow; the cap is max-h-60.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    el.style.height = '0';
    el.style.height = `${el.scrollHeight}px`;
  }, [draft]);
  useEffect(() => {
    if (focusOn) box.current?.focus();
  }, [focusOn]);
  const suggest = useSuggest(draft, setDraft, commands);
  // A command keeps its `!` in the draft, first, whatever came before it; the box shows it as the chip.
  const bash = draft.startsWith('!');
  const write = (v: string) => setDraft(/^\s+!/.test(v) ? v.trimStart() : v);
  // Typing the `!` in front of words already written hides it: the caret stays where it was, one back.
  const caret = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (caret.current === null) return;
    box.current?.setSelectionRange(caret.current, caret.current);
    caret.current = null;
  }, [bash]);
  const body = draft.trim();
  // A message goes once every image in it is up and nothing is held back; one with images may go without text. A command needs one.
  const ready = !locked && (bash ? body.slice(1).trim() : body || attach.ids.length > 0) && attach.sendable;
  const empty = !bash && !locked && !body && !attach.ids.length && attach.sendable ? onEmpty : undefined;
  const label = bash ? undefined : (empty?.label ?? sendLabel);
  const submit = () => {
    if (suggest.command(draft)) return;
    if (empty) return empty.run();
    if (!ready) return;
    onSend(body, attach.ids);
    setDraft('');
    attach.clear();
  };
  return (
    <form
      className={`relative my-3 flex flex-col gap-2 rounded-xl border bg-bg py-2 pr-2 pl-4 ${bash ? 'border-warning' : 'border-border focus-within:border-accent'} ${dropOver ? 'ring-2 ring-accent' : ''}`}
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <DropHint shown={dropOver} />
      {!bash && suggest.menu}
      {/* Held to the box's width: a long line in it truncates instead of widening the page. */}
      {above && <div className="min-w-0 overflow-hidden">{above}</div>}
      {attach.uploads.length + attach.folders.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-1">
          {attach.uploads.map((d) => (
            <UploadChip
              key={`u${d.key}`}
              draft={d}
              onOpen={d.preview ? () => setViewing(shown.indexOf(d)) : undefined}
              onRemove={() => attach.remove(d.key)}
              onRetry={() => attach.retry(d.key)}
            />
          ))}
          {viewing !== null && (
            <ImageViewer
              pictures={shown.map((d) => ({ url: d.preview, name: d.name }))}
              start={viewing}
              onClose={() => setViewing(null)}
              onMarked={
                onMarked &&
                ((marked) => {
                  for (const m of marked) {
                    const d = shown.find((x) => x.preview === m.picture.url);
                    if (d) attach.remove(d.key);
                  }
                  onMarked(marked);
                })
              }
            />
          )}
          {attach.folders.map((d) => (
            <FolderChip key={`f${d.key}`} draft={d} onRemove={() => attach.removeFolder(d.key)} onRetry={() => attach.retryFolder(d.key)} />
          ))}
        </div>
      )}
      {attach.problem && <p className="text-xs text-danger">{attach.problem}</p>}
      {suggest.refused && <p className="text-xs text-danger">{suggest.refused}</p>}
      <div className="flex items-start gap-2">
        {bash && <BashChip className="mt-0.5" />}
        <textarea
          onPaste={(e) => {
            if (!e.clipboardData.files.length) return;
            e.preventDefault();
            void readDropped(e.clipboardData).then(attach.add);
          }}
          ref={box}
          value={bash ? draft.slice(1) : draft}
          onChange={(e) => {
            const v = e.target.value;
            if (!bash && v.trimStart().startsWith('!'))
              caret.current = Math.max(0, e.target.selectionStart - 1 - (v.length - v.trimStart().length));
            write(bash ? `!${v}` : v);
          }}
          onKeyDown={(e) => {
            if (!bash && suggest.onKeyDown(e)) return;
            if (bash && (e.key === 'Escape' || (e.key === 'Backspace' && draft === '!'))) {
              e.preventDefault();
              setDraft(draft.slice(1));
              return;
            }
            if (e.key === 'Escape' && running && onStop) {
              e.preventDefault();
              onStop();
              return;
            }
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit();
            }
          }}
          rows={1}
          disabled={!!locked}
          placeholder={locked ?? (bash ? `A command for ${agent}'s terminal` : (placeholder ?? `Message ${agent}`))}
          className={`scrollbar-auto disabled:placeholder:animate-pulse max-h-60 min-h-7 flex-1 resize-none bg-transparent py-0.5 pr-2 text-fg outline-none placeholder:text-muted ${bash ? 'font-mono text-sm' : 'text-base'}`}
        />
      </div>
      <div className="-ml-2 flex items-center gap-1">
        <button
          type="button"
          aria-label="Attach files"
          title="Attach files"
          onClick={() => picker.current?.click()}
          className="grid size-7 shrink-0 place-items-center rounded-md text-muted outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
        >
          <Paperclip size={15} />
        </button>
        <input
          ref={picker}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            const files = [...(e.target.files ?? [])];
            e.target.value = '';
            attach.add(files.map((file) => ({ kind: 'file', name: file.name, files: [{ path: file.name, file }] })));
          }}
        />
        {leading}
        <span className="flex-1 truncate px-1 text-xs text-muted">{bash && 'Esc for a message'}</span>
        {trailing}
        {running && onStop && !ready ? (
          <button
            type="button"
            aria-label="Stop"
            title="Stop (Esc)"
            onClick={onStop}
            className="grid size-7 shrink-0 place-items-center rounded-md bg-fg text-bg transition-opacity outline-none hover:opacity-80 focus-visible:ring-2 focus-visible:ring-accent"
          >
            <Square size={12} fill="currentColor" />
          </button>
        ) : (
          <button
            type="submit"
            aria-label={bash ? 'Run' : (label ?? 'Send')}
            title={bash ? 'Run (Enter)' : (label ?? 'Send')}
            disabled={!ready && !empty}
            className={`flex h-7 shrink-0 items-center justify-center gap-1 rounded-md transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-30 ${label ? 'px-2.5 text-sm font-medium' : 'w-7'} ${bash ? 'bg-warning text-bg' : 'bg-accent text-accent-fg'}`}
          >
            {bash ? <CornerDownLeft size={15} /> : <ArrowUp size={16} />}
            {label}
          </button>
        )}
      </div>
    </form>
  );
}

/** The mark of a command run in the agent's terminal: in the box while one is written, and on the line it leaves in the chat. */
export function BashChip({ className = '' }: { className?: string }) {
  return (
    <span className={`flex h-6 shrink-0 items-center gap-1 rounded-md bg-warning/15 px-1.5 text-xs font-medium text-warning ${className}`}>
      <SquareTerminal size={13} />
      Bash
    </span>
  );
}
