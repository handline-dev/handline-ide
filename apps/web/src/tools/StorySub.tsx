// A story's sub-task opened from its node: back to the Plan, the next one for you, and its Plan, Code and Result with the founder's verdicts.
import type { MemoFile, TaskDetail } from '@handline/client';
import { hooks, useAttachmentUrl, useNameOf, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { Check, ChevronLeft, ChevronRight, Play } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { playVideo } from '#src/viewer/VideoPlayer.tsx';
import { Changes, diffCounts, useBranchDiff } from '#src/changes/Changes.tsx';
import { clock } from '#src/format/time.ts';
import { useTabState } from '#src/persist/tabState.ts';
import { Avatar } from '#src/tools/TaskEvents.tsx';
import { startsAfter, wordOf, type Sub } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

type Tab = 'plan' | 'code' | 'result';
const CLOSED = ['done', 'discarded'];

export function SubTask({ sub, story, onPick }: { sub: Sub; story: TaskDetail; onPick: (id: number | null) => void }) {
  const t = hooks.useTaskGet({ task: sub.id }).data;
  const me = useMe();
  const nameOf = useNameOf();
  const [tab, setTab] = useTabState<Tab | null>(`subtab:${sub.id}`, null);
  if (!t) return <div className="flex-1" />;
  const open = new Set(story.subtasks.filter((x) => !CLOSED.includes(x.state)).map((x) => x.id));
  const word = wordOf(t, sub, me, open);
  // The next sub-task for you after this one, else the next one.
  const rest = story.subtasks.filter((x) => x.id !== sub.id);
  const after = [...rest.filter((x) => x.id > sub.id), ...rest.filter((x) => x.id < sub.id)];
  const next = after.find((x) => x.review?.ready || (x.state === 'draft' && x.plan_state === 'written')) ?? after[0];
  const code = !!t.branch?.exists;
  const first: Tab = t.review ? 'result' : 'plan';
  const shown: Tab = tab === 'code' && !code ? first : (tab ?? first);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4 text-sm">
        <button
          type="button"
          onClick={() => onPick(null)}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-muted hover:bg-hover/30 hover:text-fg"
        >
          <ChevronLeft size={14} />
          Plan
        </button>
        {next && (
          <button
            type="button"
            onClick={() => onPick(next.id)}
            className="flex min-w-0 items-center gap-1 rounded-md px-2 py-1 text-muted hover:bg-hover/30 hover:text-fg"
          >
            <span className="truncate">
              Next: T{next.id} {next.title}
            </span>
            <ChevronRight size={14} className="shrink-0" />
          </button>
        )}
      </div>
      <div className="flex shrink-0 flex-col gap-2 px-6 pt-5">
        <h2 className="text-lg font-semibold text-fg">{t.title}</h2>
        <p className="flex items-center gap-2 text-sm text-muted">
          <Avatar name={nameOf(t.to)} mine={false} />
          <span className="text-fg">{nameOf(t.to)}</span>
          <span>· T{t.id}</span>
          <span className={word.kind === 'you' ? 'text-warning' : ''}>· {word.text}</span>
        </p>
        <div role="tablist" className="mt-2 flex gap-4 border-b border-border text-sm">
          {(
            [
              ['plan', 'Plan', true],
              ['code', 'Code', code],
              ['result', 'Result', !!t.review],
            ] as const
          ).map(([k, label, live]) => (
            <button
              key={k}
              type="button"
              role="tab"
              aria-selected={shown === k}
              disabled={!live}
              onClick={() => setTab(k)}
              className={`-mb-px border-b-2 py-2 disabled:opacity-40 ${shown === k ? 'border-accent font-medium text-fg' : 'border-transparent text-muted hover:text-fg'}`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      {shown === 'plan' && <PlanTab t={t} open={open} />}
      {shown === 'code' && t.branch?.exists && (
        <div className="flex min-h-0 flex-1 flex-col">
          <Changes agent={t.to} branch={t.branch.name} />
        </div>
      )}
      {shown === 'result' && t.review && <ResultTab t={t} onCode={() => setTab('code')} />}
      {t.review?.ready ? <Verdict t={t} /> : t.state === 'draft' && <DraftButtons t={t} />}
    </div>
  );
}

function Footer({ children }: { children: ReactNode }) {
  return <div className="flex shrink-0 items-center justify-end gap-2 border-t border-border px-6 py-3">{children}</div>;
}

/** A Ready task's buttons, on every tab: Redo with a note, or OK, merge. */
function Verdict({ t }: { t: TaskDetail }) {
  const ok = useWrite('task.ok');
  const redo = useWrite('task.redo');
  return (
    <Footer>
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
        OK, merge
      </Button>
    </Footer>
  );
}

/** A draft's buttons, before Run: ask for a plan or its core logic, and Plan OK. */
function DraftButtons({ t }: { t: TaskDetail }) {
  const ask = useWrite('task.plan_ask');
  const core = useWrite('task.core_ask');
  const plan = t.plan;
  return (
    <Footer>
      {!plan && t.plan_state === 'none' && (
        <Button variant="primary" disabled={ask.isPending} onClick={() => ask.mutate({ task: t.id }, { onError: sayRefusal })}>
          Ask for a plan
        </Button>
      )}
      {plan && (
        <Button
          disabled={core.isPending || (!!t.core_ask && !t.core_ask.sent_at)}
          onClick={() =>
            void note({ title: 'Ask for core logic', hint: 'Which part, if any (optional)', go: 'Ask', required: false }).then(
              (n) => n !== undefined && core.mutate({ task: t.id, ...(n ? { note: n } : {}) }, { onError: sayRefusal }),
            )
          }
        >
          Ask for core logic
        </Button>
      )}
      <PlanOk t={t} />
    </Footer>
  );
}

/** Plan OK, or, once given, the OK and a way to take it back. */
function PlanOk({ t }: { t: TaskDetail }) {
  const ok = useWrite('task.plan_ok');
  if (!t.plan) return null;
  if (t.plan.ok)
    return (
      <>
        <Button disabled={ok.isPending} onClick={() => ok.mutate({ task: t.id, ok: false }, { onError: sayRefusal })}>
          Undo Plan OK
        </Button>
        <span className="flex items-center gap-1 text-sm text-success">
          <Check size={14} />
          Plan OK
        </span>
      </>
    );
  return (
    <Button
      variant="primary"
      disabled={ok.isPending}
      onClick={() => ok.mutate({ task: t.id, version: t.plan!.version }, { onError: sayRefusal })}
    >
      Plan OK
    </Button>
  );
}

function PlanTab({ t, open }: { t: TaskDetail; open: Set<number> }) {
  const nameOf = useNameOf();
  const plan = t.plan;
  const before = new Set(plan && plan.versions.length > 1 ? plan.versions.at(-2)!.text.split('\n') : []);
  const changed = plan && plan.versions.length > 1;
  const after = startsAfter(t, open);
  return (
    <ScrollFade className="min-h-0 flex-1">
      <div className="flex flex-col gap-4 px-6 py-5 text-sm">
        {!plan ? (
          <p className="text-muted">
            {t.plan_state === 'asked' || t.plan_state === 'writing' ? `${nameOf(t.to)} is writing the plan…` : 'No plan yet.'}
          </p>
        ) : (
          <>
            <p className="text-xs text-muted">
              {changed ? `Version ${plan.version} · updated ${clock(plan.at)}` : `Written ${clock(plan.at)}`}
              {plan.ok && ` · OK ${clock(plan.ok.at)}`}
            </p>
            <div className="flex flex-col">
              {plan.text.split('\n').map((line, i) => {
                const fresh = changed && line.trim() !== '' && !before.has(line);
                return (
                  <p
                    key={i}
                    className={`min-h-5 border-l-2 py-0.5 pl-3 whitespace-pre-wrap ${fresh ? 'border-accent bg-accent/10' : 'border-transparent'}`}
                  >
                    {line}
                  </p>
                );
              })}
            </div>
            {changed && <p className="text-xs text-muted">Lines marked on the left changed since version {plan.version - 1}.</p>}
          </>
        )}
        <p className="text-muted">
          Starts after: <span className="text-fg">{after.length ? after.map((n) => `T${n}`).join(', ') : 'nothing, starts at once'}</span>
        </p>
        {t.core_ask && (
          <p className="text-muted">
            Core logic asked {clock(t.core_ask.at)}
            {t.core_ask.sent_at ? '' : ', sent once the agent is free'}
            {t.core_ask.note && `: ${t.core_ask.note}`}
          </p>
        )}
      </div>
    </ScrollFade>
  );
}

function ResultTab({ t, onCode }: { t: TaskDetail; onCode: () => void }) {
  const review = t.review!;
  const memo = t.memos.find((m) => m.id === review.memo);
  const files = memo?.files ?? [];
  const posters = new Set(files.map((f) => f.poster).filter(Boolean));
  const pictures = files.filter((f) => f.kind === 'image' && !posters.has(f.id));
  const videos = files.filter((f) => f.kind === 'video');
  const diff = useBranchDiff(t.to, t.branch?.exists ? t.branch.name : null).data;
  const changed = diff ? diffCounts(diff.diff) : [];
  const sum = (k: 'added' | 'deleted') => changed.reduce((n, f) => n + f[k], 0);
  return (
    <ScrollFade className="min-h-0 flex-1">
      <div className="flex flex-col gap-5 px-6 py-5 text-sm">
        <div className="grid grid-cols-3 gap-3">
          {pictures.map((f) => (
            <Framed key={f.id} f={f} />
          ))}
          {videos.map((f) => (
            <Video key={f.id} f={f} />
          ))}
        </div>
        <button type="button" onClick={onCode} className="self-start text-muted hover:text-fg hover:underline">
          {changed.length} files · +{sum('added')} −{sum('deleted')}
        </button>
        {memo && (
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted">Changed from the plan</span>
            <Markdown text={memo.text} />
          </div>
        )}
        {review.redo > 0 && <p className="text-xs text-muted">Sent back {review.redo === 1 ? 'once' : `${review.redo} times`}.</p>}
        {!review.ready && <p className="text-xs text-muted">{review.ok ? 'OK given; merging.' : 'Sent back; being worked again.'}</p>}
      </div>
    </ScrollFade>
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

/** A note for an ask: resolves the text (maybe empty when not required), or undefined when cancelled. */
function note({ title, hint, go, required }: { title: string; hint: string; go: string; required: boolean }) {
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
