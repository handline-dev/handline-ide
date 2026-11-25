import { count } from '#src/format/count.ts';
import type { TurnRecord as Turn } from '@handline/app-core';
import type { TranscriptItem } from '@handline/client';
import { commandGist } from '@handline/app-core';
import { motion } from 'framer-motion';
import { ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { clock } from '#src/format/time.ts';
import { Ago } from '#src/ui/Ago.tsx';
import { spring } from '#src/motion.ts';
import { Markdown } from '#src/ui/Markdown.tsx';

/** A turn's tool calls and the agent's own text, folded into one muted line ("Read docs/plan.md ›", "Ran 3 steps ›"), as Claude Code
 *  folds them: no card; a click opens them in order under it. */
export function TurnRecord({ r, fade }: { r: Turn; fade: boolean }) {
  const [open, setOpen] = useState(false);
  const notes = r.items.filter((i) => i.kind === 'reply').length;
  const steps = r.items.length - notes; // a permission ask is a step as a tool call is
  // A permission ask still open says so on the folded line: it waits on the person, not on the agent.
  const asks = r.items.some((i) => i.kind === 'prompt' && i.open);
  const only = steps === 1 ? r.items.find((i) => i.kind !== 'reply') : undefined;
  const what = only ? oneLine(only) : steps ? `Ran ${count(steps, 'step')}` : count(notes, 'note');
  return (
    <motion.div
      data-id={`record:${r.id}`}
      initial={fade ? { opacity: 0 } : false}
      animate={{ opacity: 1 }}
      transition={spring}
      className="group flex flex-col items-start"
    >
      <div className="w-full max-w-150 text-sm">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="flex max-w-full items-center gap-1 py-0.5 text-left text-muted hover:text-fg"
        >
          {asks && <span className="shrink-0 text-warning">Asks your permission ·</span>}
          <span className="min-w-0 truncate">{what}</span>
          <ChevronRight size={14} className={`shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
        </button>
        {open && (
          <div className="mt-1 ml-1 flex flex-col gap-1.5 border-l border-border pl-3">
            {r.items.map((i) => (
              <Step key={i.id} i={i} />
            ))}
          </div>
        )}
      </div>
      <time
        dateTime={r.until}
        title={clock(r.at) === clock(r.until) ? clock(r.at) : `${clock(r.at)} ~ ${clock(r.until)}`}
        className="mb-2 flex h-6 items-center text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        <Ago at={r.until} />
      </time>
    </motion.div>
  );
}

/** One step as the folded line says it: the tool and what it touched, a permission ask as what it asked. */
function oneLine(i: TranscriptItem): string {
  if (i.kind === 'prompt') return `Asked to use ${i.prompt?.tool ?? 'a tool'}${i.prompt?.command ? `: ${i.prompt.command}` : ''}`;
  return [i.tool, i.text].filter(Boolean).join(' ');
}

/** A tool call: its tool, then what it ran in mono, wrapped, never cut mid-path. A permission ask: who it waits on or how it ended, and
 *  the command, with what it does in plain words when its first word says so. The agent's text as it wrote it. */
export function Step({ i }: { i: TranscriptItem }) {
  if (i.kind === 'reply') return <div className="min-w-0 text-sm text-muted">{i.text && <Markdown text={i.text} />}</div>;
  if (i.kind === 'prompt') {
    const command = i.prompt?.command ?? null;
    const gist = commandGist(command);
    const a = i.answer;
    const ended = i.open
      ? 'waiting for you'
      : a && 'label' in a && a.label
        ? `${a.label}${a.by ? ` by ${a.by}` : ''}`
        : 'answered in the terminal';
    return (
      <div className="flex min-w-0 flex-col gap-1">
        <p className="text-xs text-muted">
          <span className={i.open ? 'text-warning' : 'text-fg'}>Asked to use {i.prompt?.tool ?? 'a tool'}</span>
          {gist && ` · ${gist}`} · {ended}
        </p>
        {command && <Code text={command} />}
      </div>
    );
  }
  return (
    <div className="flex min-w-0 items-baseline gap-2">
      {i.tool && <span className="shrink-0 text-xs text-muted">{i.tool}</span>}
      {i.text && <Code text={i.text} />}
    </div>
  );
}

/** What a step ran, in mono: whole and wrapped at three lines, the rest on hover. */
function Code({ text }: { text: string }) {
  return (
    <code title={text} className="line-clamp-3 min-w-0 font-mono text-xs break-all whitespace-pre-wrap text-fg">
      {text}
    </code>
  );
}
