import type { TranscriptItem } from '@handline/client';
import { Check, ChevronRight } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { count } from '#src/format/count.ts';
import { ago, useNow } from '#src/format/time.ts';
import { Step } from '#src/chat/TurnRecord.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';

/** A Chat agent's turn, which has no terminal to look at: each step on its own line as it lands, and a click opens what it did. A
 *  subagent's steps sit under the call that started it. */
export function StepRows({ items }: { items: TranscriptItem[] }) {
  const ids = new Set(items.map((i) => i.id));
  const under = new Map<number, TranscriptItem[]>();
  for (const i of items) if (i.parent != null && ids.has(i.parent)) under.set(i.parent, [...(under.get(i.parent) ?? []), i]);
  const top = items.filter((i) => i.parent == null || !ids.has(i.parent));
  return (
    <div className="mb-2 flex w-full max-w-150 min-w-0 flex-col gap-0.5 text-sm">
      <Rows items={top} under={under} />
    </div>
  );
}

function Rows({ items, under }: { items: TranscriptItem[]; under: Map<number, TranscriptItem[]> }) {
  return items.map((i) =>
    i.kind === 'reply' ? <Said key={i.id} text={i.text} /> : <StepRow key={i.id} i={i} kids={under.get(i.id) ?? []} under={under} />,
  );
}

function Said({ text }: { text?: string }) {
  return text ? <div className="min-w-0 py-1 text-fg">{<Markdown text={text} />}</div> : null;
}

function StepRow({ i, kids, under }: { i: TranscriptItem; kids: TranscriptItem[]; under: Map<number, TranscriptItem[]> }) {
  const [open, setOpen] = useState(false);
  const now = useNow(i.running ? 1000 : null);
  if (i.kind !== 'tool' && i.kind !== 'thinking') return <Step i={i} />;
  const thinking = i.kind === 'thinking';
  const out = thinking ? '' : (i.output ?? '');
  const body: ReactNode =
    (thinking && i.text) || kids.length || i.diff || out ? (
      <>
        {thinking && <p className="text-muted italic">{i.text}</p>}
        {kids.length > 0 && (
          <div className="flex flex-col gap-0.5">
            <Rows items={kids} under={under} />
          </div>
        )}
        {i.diff ? <Diff d={i.diff} /> : out && <Output text={out} dropped={i.dropped_lines} />}
      </>
    ) : null;
  const meta = i.running
    ? `running ${ago(i.at, now)}`
    : kids.length
      ? count(kids.filter((k) => k.kind !== 'reply').length, 'step')
      : i.diff
        ? `+${i.added ?? 0} −${i.removed ?? 0}`
        : '';
  return (
    <div className="flex min-w-0 flex-col">
      <button
        type="button"
        aria-expanded={open}
        disabled={!body}
        onClick={() => setOpen(!open)}
        className="flex min-w-0 items-baseline gap-2 py-0.5 text-left text-muted enabled:hover:text-fg"
      >
        {i.running ? (
          <span aria-label="running" className="size-2 shrink-0 self-center rounded-full bg-warning" />
        ) : (
          <Check size={12} aria-label="done" className="shrink-0 self-center text-success" />
        )}
        <span className="shrink-0 font-medium text-fg">{thinking ? 'Thinking' : i.tool}</span>
        {!thinking && i.text && <code className="min-w-0 truncate font-mono text-xs text-fg">{i.text}</code>}
        {meta && <span className="shrink-0 text-xs tabular-nums">· {meta}</span>}
        {body && <ChevronRight size={14} className={`shrink-0 self-center transition-transform ${open ? 'rotate-90' : ''}`} />}
      </button>
      {open && body && <div className="mt-1 mb-1.5 ml-1.5 flex min-w-0 flex-col gap-1.5 border-l border-border pl-3">{body}</div>}
    </div>
  );
}

const block = 'overflow-x-auto rounded bg-surface px-2.5 py-2 font-mono text-xs whitespace-pre';

function Output({ text, dropped }: { text: string; dropped?: number }) {
  return (
    <div>
      <pre className={`${block} text-fg`}>{text}</pre>
      {!!dropped && <p className="mt-1 text-xs text-muted">{dropped} more lines not kept</p>}
    </div>
  );
}

/** An edit's change, line by line: added green, removed red, the rest as it was. */
function Diff({ d }: { d: NonNullable<TranscriptItem['diff']> }) {
  return (
    <div>
      <pre className={block}>
        {d.hunks.flatMap((h, n) => [
          ...(n > 0
            ? [
                <span key={`gap${n}`} className="block text-muted">
                  ⋯
                </span>,
              ]
            : []),
          ...h.lines.map((l, k) => (
            <span
              key={`${n}:${k}`}
              className={`block ${l.startsWith('+') ? 'text-success' : l.startsWith('-') ? 'text-danger' : 'text-muted'}`}
            >
              {l}
            </span>
          )),
        ])}
      </pre>
      {!!d.dropped_lines && <p className="mt-1 text-xs text-muted">{d.dropped_lines} more lines not kept</p>}
    </div>
  );
}
