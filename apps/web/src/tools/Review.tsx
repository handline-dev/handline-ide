// A Ready task's result and the founder's verdict on it: shared by a story's sub-task panel and a task's own page.
import type { MemoFile, TaskDetail } from '@handline/client';
import { useAttachmentUrl, useWrite } from '@handline/app-data';
import { Play } from 'lucide-react';
import { useState } from 'react';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { ChangeSize, TaskCode } from '#src/tools/TaskCode.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { playVideo } from '#src/viewer/VideoPlayer.tsx';

/** A Ready task's result, in the order the founder judges it: the pictures or video, the code, then what changed from the plan. */
export function Result({ t, review }: { t: TaskDetail; review: NonNullable<TaskDetail['review']> }) {
  const memo = t.memos.find((m) => m.id === review.memo);
  const files = memo?.files ?? [];
  const posters = new Set(files.map((f) => f.poster).filter(Boolean));
  const pictures = files.filter((f) => f.kind === 'image' && !posters.has(f.id));
  const videos = files.filter((f) => f.kind === 'video');
  const others = files.filter((f) => f.kind !== 'image' && f.kind !== 'video');
  return (
    <div className="flex flex-col gap-5 text-sm">
      <div className="grid grid-cols-3 gap-3">
        {pictures.map((f) => (
          <Framed key={f.id} f={f} />
        ))}
        {videos.map((f) => (
          <Video key={f.id} f={f} />
        ))}
      </div>
      <MessageImages images={others.map((f) => ({ id: f.id, name: f.name, mime: f.mime, size: f.bytes }))} />
      <details className="rounded-md border border-border">
        <summary className="flex cursor-pointer items-center gap-2 px-3 py-2 text-xs text-muted">
          Code <ChangeSize task={t.id} />
        </summary>
        <TaskCode task={t.id} />
      </details>
      {memo && (
        <div className="flex flex-col gap-1">
          <span className="text-xs text-muted">Changed from the plan</span>
          <Markdown text={memo.text} />
        </div>
      )}
      {review.redo > 0 && <p className="text-xs text-muted">Sent back {review.redo === 1 ? 'once' : `${review.redo} times`}.</p>}
      {!review.ready && <p className="text-xs text-muted">{review.ok ? 'OK given; merging.' : 'Sent back; being worked again.'}</p>}
    </div>
  );
}

/** A picture framed as one, under a window bar and a little faded, so its buttons do not look live. */
function Framed({ f }: { f: MemoFile }) {
  const { url } = useAttachmentUrl(f.id, false);
  return (
    <figure className="flex flex-col gap-1">
      <div className="overflow-hidden rounded-md border border-border">
        <div className="flex h-5 items-center gap-1 border-b border-border bg-surface px-2">
          {[0, 1, 2].map((i) => (
            <span key={i} className="size-1.5 rounded-full bg-muted/50" />
          ))}
        </div>
        {url ? (
          <img src={url} alt={f.name} className="h-32 w-full object-cover object-top opacity-80" />
        ) : (
          <div className="h-32 bg-hover/30" />
        )}
      </div>
      <figcaption className="text-xs text-muted">{f.name.replace(/\.\w+$/, '')}</figcaption>
    </figure>
  );
}

function Video({ f }: { f: MemoFile }) {
  return (
    <figure className="flex flex-col gap-1">
      <button type="button" onClick={() => playVideo(f.id, f.name)} className="relative overflow-hidden rounded-md border border-border">
        {f.poster ? <Poster id={f.poster} /> : <div className="h-37 bg-hover/30" />}
        <span className="absolute inset-0 m-auto flex size-12 items-center justify-center rounded-full bg-fg/60 text-bg">
          <Play size={20} />
        </span>
      </button>
      <figcaption className="text-xs text-muted">{f.name}</figcaption>
    </figure>
  );
}

function Poster({ id }: { id: string }) {
  const url = useAttachmentUrl(id, false).url;
  return url ? <img src={url} alt="" className="h-37 w-full object-cover object-top opacity-80" /> : <div className="h-37 bg-hover/30" />;
}

/** Redo with a note, or OK: a Ready task's verdict. */
export function VerdictButtons({ t }: { t: TaskDetail }) {
  const ok = useWrite('task.ok');
  const redo = useWrite('task.redo');
  return (
    <span className="flex shrink-0 gap-2">
      <Button
        disabled={redo.isPending}
        onClick={() =>
          void note({ title: 'Redo', hint: 'What to change', go: 'Send back', required: true }).then(
            (n) => n && redo.mutate({ task: t.id, note: n }, { onError: sayRefusal }),
          )
        }
      >
        Redo
      </Button>
      <Button variant="primary" disabled={ok.isPending} onClick={() => ok.mutate({ task: t.id }, { onError: sayRefusal })}>
        OK
      </Button>
    </span>
  );
}

/** A note for an ask: resolves the text (maybe empty when not required), or undefined when cancelled. */
export function note({ title, hint, go, required }: { title: string; hint: string; go: string; required: boolean }) {
  return openModal<string>((close) => <NoteWindow title={title} hint={hint} go={go} required={required} close={close} />);
}

function NoteWindow({
  title,
  hint,
  go,
  required,
  close,
}: {
  title: string;
  hint: string;
  go: string;
  required: boolean;
  close: (v?: string) => void;
}) {
  const [text, setText] = useState('');
  return (
    <ModalFrame
      title={title}
      onClose={() => close()}
      footer={
        <>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="primary" disabled={required && !text.trim()} onClick={() => close(text.trim())}>
            {go}
          </Button>
        </>
      }
    >
      <textarea
        autoFocus
        rows={4}
        value={text}
        placeholder={hint}
        onChange={(e) => setText(e.target.value)}
        className="w-full rounded-md border border-border bg-bg p-2 text-sm text-fg outline-none placeholder:text-muted focus:border-accent"
      />
    </ModalFrame>
  );
}
