import { ArrowUp, Paperclip, Square } from 'lucide-react';
import { useLayoutEffect, useRef, type ReactNode } from 'react';
import { FolderChip, UploadChip, type Attachments } from '#src/chat/attachments.tsx';
import { readDropped } from '#src/ui/fileDrop.tsx';

/** The box a message is written in: it grows with the text up to a cap, then scrolls; Enter sends, Shift+Enter breaks a line.
 * Under the text, a row: attach and `leading` on the left (the model), `trailing` and send on the right (the context). While the
 * agent works and nothing is written, send is Stop; Esc in the box stops it too, as in a terminal agent. */
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
}: {
  agent: string;
  draft: string;
  setDraft: (v: string) => void;
  attach: Attachments;
  onSend: (body: string, attachments: string[]) => void;
  leading?: ReactNode;
  trailing?: ReactNode;
  running?: boolean;
  onStop?: () => void;
  /** Why the box takes no message now (a compaction under way), shown in its place; absent when it takes one. */
  locked?: string;
}) {
  const picker = useRef<HTMLInputElement>(null);
  const box = useRef<HTMLTextAreaElement>(null);
  // Height follows the text: read it at no height, so it can shrink as well as grow; the cap is max-h-60.
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    el.style.height = '0';
    el.style.height = `${el.scrollHeight}px`;
  }, [draft]);
  const body = draft.trim();
  // A message goes once every image in it is up and nothing is held back; one with images may go without text.
  const ready = !locked && (body || attach.ids.length > 0) && attach.sendable;
  const submit = () => {
    if (!ready) return;
    onSend(body, attach.ids);
    setDraft('');
    attach.clear();
  };
  return (
    <form
      className="my-3 flex flex-col gap-2 rounded-xl border border-border bg-bg py-2 pr-2 pl-4 focus-within:border-accent"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      {attach.uploads.length + attach.folders.length > 0 && (
        <div className="flex flex-wrap gap-2 pt-1">
          {attach.uploads.map((d) => (
            <UploadChip key={`u${d.key}`} draft={d} onRemove={() => attach.remove(d.key)} onRetry={() => attach.retry(d.key)} />
          ))}
          {attach.folders.map((d) => (
            <FolderChip key={`f${d.key}`} draft={d} onRemove={() => attach.removeFolder(d.key)} />
          ))}
        </div>
      )}
      {attach.problem && <p className="text-xs text-danger">{attach.problem}</p>}
      <textarea
        onPaste={(e) => {
          if (!e.clipboardData.files.length) return;
          e.preventDefault();
          void readDropped(e.clipboardData).then(attach.add);
        }}
        ref={box}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
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
        placeholder={locked ?? `Message ${agent}`}
        className="scrollbar-auto disabled:placeholder:animate-pulse max-h-60 min-h-7 resize-none bg-transparent py-0.5 pr-2 text-base text-fg outline-none placeholder:text-muted"
      />
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
        <span className="flex-1" />
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
            aria-label="Send"
            title="Send"
            disabled={!ready}
            className="grid size-7 shrink-0 place-items-center rounded-md bg-accent text-accent-fg transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-30"
          >
            <ArrowUp size={16} />
          </button>
        )}
      </div>
    </form>
  );
}
