import { ArrowUp } from 'lucide-react';
import { useLayoutEffect, useRef } from 'react';
import { FolderChip, UploadChip, type Attachments } from '#src/chat/attachments.tsx';
import { readDropped } from '#src/ui/fileDrop.tsx';

/** The box a message is written in: it grows with the text up to a cap, then scrolls; Enter sends, Shift+Enter breaks a line. */
export function Composer({
  agent,
  draft,
  setDraft,
  attach,
  onSend,
}: {
  agent: string;
  draft: string;
  setDraft: (v: string) => void;
  attach: Attachments;
  onSend: (body: string, attachments: string[]) => void;
}) {
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
  const ready = (body || attach.ids.length > 0) && attach.sendable;
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
      <div className="flex items-end gap-2">
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
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit();
            }
          }}
          rows={1}
          placeholder={`Message ${agent}`}
          className="scrollbar-auto max-h-60 min-h-7 flex-1 resize-none bg-transparent py-0.5 text-base text-fg outline-none placeholder:text-muted"
        />
        <button
          type="submit"
          aria-label="Send"
          title="Send"
          disabled={!ready}
          className="grid size-7 shrink-0 place-items-center rounded-md bg-accent text-accent-fg transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-30"
        >
          <ArrowUp size={16} />
        </button>
      </div>
    </form>
  );
}
