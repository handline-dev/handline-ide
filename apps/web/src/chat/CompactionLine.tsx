import { Ago } from '#src/ui/Ago.tsx';
import { motion } from 'framer-motion';
import { ChevronRight, Clock, FileText, FoldVertical, RotateCw } from 'lucide-react';
import { type ReactNode, useState } from 'react';
import { spring } from '#src/motion.ts';
import { Markdown } from '#src/ui/Markdown.tsx';
import type { LineChange } from '@handline/client';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { textOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { names, write } from '#src/persist/kv.ts';
import { textKey } from '#src/tools/TextTab.tsx';

/** Why a conversation began or restarted, as the line says it; a word the daemon adds later shows as it is. */
const causes: Record<string, string> = {
  wake: '', // a wake gives no reason; an expired cache is recorded as idle_close or fresh close
  launch: 'first start',
  start: 'new work arrived',
  restart: 'restarted',
  'fresh close': 'cache expired',
  idle_close: 'cache expired',
  move: 'moved to another account',
  handoff: 'context was getting full',
  context_full: 'context was full',
  settings_applied: 'settings changed',
  crash_recovery: 'recovered from a crash',
};
const causeWords = (cause?: string) => (cause ? (causes[cause] ?? cause.replaceAll('_', ' ')) : null);

/** The daemon's `changed`, one change each in plain words: split at the commas outside its brackets, the bracket (file names) dropped, said once. */
export function changesOf(changed?: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let cur = '';
  for (const ch of changed ?? '') {
    if (ch === ',' && depth === 0) {
      out.push(cur.trim());
      cur = '';
      continue;
    }
    if (ch === '(') depth++;
    if (ch === ')') depth = Math.max(0, depth - 1);
    cur += ch;
  }
  out.push(cur.trim());
  return [...new Set(out.map((c) => c.replace(/\s*\(.*$/, '')).filter(Boolean))];
}

/** The line's words: a new conversation is Compacted and says why; a restart into the same one says what changed in short (one change
 * or several as settings), else why (its own cause `restart` adds nothing). The full changes are behind a click. */
export function compactionLabel(s: { cause?: string; resumed?: boolean; changed?: string }): string {
  const why = causeWords(s.cause);
  if (!s.resumed) return `Compacted${why ? `: ${why}` : ''}`;
  const changes = changesOf(s.changed);
  const what = changes.length > 1 ? 'settings changed' : (changes[0] ?? (s.cause === 'restart' ? null : why));
  return `Restarted${what ? `: ${what}` : ''}`;
}

/** Where a new conversation of the agent began, or it restarted into the same one: a centred line between rules, with its handoff note when it had one. */
export function CompactionLine({
  at,
  cause,
  handoff,
  handoff_note: note,
  resumed,
  changed,
  changed_lines: lines = [],
}: {
  at: string;
  cause?: string;
  handoff: string | null;
  handoff_note?: string | null;
  resumed?: boolean;
  changed?: string;
  changed_lines?: LineChange[];
}) {
  const why = causeWords(cause);
  const [shown, setShown] = useState(false);
  const hover = {
    onPointerEnter: () => setShown(true),
    onPointerLeave: () => setShown(false),
    onFocus: () => setShown(true),
    onBlur: () => setShown(false),
  };
  const box = 'flex flex-col items-center rounded-md px-2 py-0.5 transition-colors outline-none';
  const label = compactionLabel({ ...(cause ? { cause } : {}), ...(resumed ? { resumed } : {}), ...(changed ? { changed } : {}) });
  const Icon = resumed ? RotateCw : FoldVertical;
  // The summary it was given is there for whoever wants it: the label opens it, nothing else points at it.
  const body = (
    <>
      <span className="flex items-center gap-1.5">
        <Icon size={14} />
        {label}
      </span>
      <motion.time
        dateTime={at}
        initial={false}
        animate={shown ? { height: 'auto', opacity: 1 } : { height: 0, opacity: 0 }}
        transition={spring}
        className="overflow-hidden text-xs"
      >
        <Ago at={at} />
      </motion.time>
    </>
  );
  const changes = changesOf(changed);
  const open = () =>
    void openModal((close) =>
      resumed ? (
        <ModalFrame title="Restarted" onClose={() => close()}>
          <Restart at={at} changes={changes} lines={lines} note={note} close={() => close()} />
        </ModalFrame>
      ) : (
        <ModalFrame title="Context compacted" size="xl" onClose={() => close()}>
          <Compaction at={at} why={why} handoff={handoff ?? ''} />
        </ModalFrame>
      ),
    );
  return (
    // On hover the time opens under the label, inside its hover box, from no height to its own. The row
    // holds the open box's height (label 20 + time 16 + padding 4), so the chat never moves; all is centred in it.
    <div className="my-3 flex h-10 items-center gap-4 text-sm text-muted">
      <div className="h-px flex-1 bg-border" />
      {(resumed ? changes.length || note !== undefined : handoff) ? (
        <button
          type="button"
          onClick={open}
          {...hover}
          className={`${box} hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent`}
        >
          {body}
        </button>
      ) : (
        <div {...hover} className={box}>
          {body}
        </div>
      )}
      <div className="h-px flex-1 bg-border" />
    </div>
  );
}

const FIRST = 8;

/** One part's removed (−) and added (+) lines; Show all opens its whole text beside, or unfolds the rest here. */
function PartLines({ l, at, close }: { l: LineChange; at: string; close: () => void }) {
  const [all, setAll] = useState(false);
  const opens = textOf(at, l.part);
  // Show all opens the whole text the way any link opens, after keeping it: only the latest, each up to 256 KiB.
  const whole = useOpenable(opens, {
    focus: () => {
      const key = textKey(opens.slice(5));
      for (const n of names(textKey(''))) if (n !== key) write(n, undefined);
      write(key, l.text);
    },
    onOpen: close,
  });
  const rows = [...l.removed.map((t) => ({ t, add: false })), ...l.added.map((t) => ({ t, add: true }))];
  const shown = all ? rows : rows.slice(0, FIRST);
  const hidden = rows.length - shown.length;
  const more =
    l.text != null
      ? { open: whole, label: 'Show all' }
      : hidden > 0
        ? { open: { onClick: () => setAll(true) }, label: `Show all ${rows.length + (l.more ?? 0)} lines` }
        : null;
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <div className="flex items-center gap-2 border-b border-border bg-hover/40 px-3 py-1.5 text-xs">
        <FileText size={13} className="shrink-0 text-muted" />
        <span className="min-w-0 flex-1 truncate font-mono text-fg">{l.part}</span>
        <span className="font-mono text-success">+{l.added.length}</span>
        <span className="font-mono text-danger">−{l.removed.length}</span>
      </div>
      <pre className="max-h-96 overflow-auto py-1 font-mono text-xs leading-5 whitespace-pre-wrap">
        {shown.map((r, i) => (
          <div key={i} className={`flex px-3 ${r.add ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}`}>
            <span className="w-4 shrink-0 select-none">{r.add ? '+' : '−'}</span>
            <span className="min-w-0">{r.t}</span>
          </div>
        ))}
        {!!l.more && <div className="px-3 text-muted">and {l.more} more lines, too many to keep</div>}
      </pre>
      {more && (
        <button
          type="button"
          {...more.open}
          className="w-full border-t border-border px-3 py-1.5 text-left text-xs text-accent hover:bg-hover/40"
        >
          {more.label}
        </button>
      )}
    </div>
  );
}

function Heading({ children }: { children: ReactNode }) {
  return <h3 className="text-xs font-semibold tracking-wide text-muted uppercase">{children}</h3>;
}

/** What a restart changed, then the agent's handoff note, or that it wrote none. */
function Restart({
  at,
  changes,
  lines,
  note,
  close,
}: {
  at: string;
  changes: string[];
  lines: LineChange[];
  note: string | null | undefined;
  close: () => void;
}) {
  const shown = new Set(lines.map((l) => l.part));
  // A change the daemon kept no lines for (a model, or an older daemon) stays a plain line.
  const plain = changes.filter((c) => ![...shown].some((p) => c === `${p} changed` || c.startsWith(`${p} changed `)));
  return (
    <div className="flex flex-col gap-6 pb-1">
      <div className="flex items-center gap-2 text-sm text-muted">
        <Clock size={14} className="shrink-0" />
        <span>When</span>
        <span className="text-fg">{new Date(at).toLocaleString()}</span>
      </div>
      {changes.length > 0 && (
        <section className="flex flex-col gap-2">
          <Heading>What changed</Heading>
          {lines.map((l) => (
            <PartLines key={l.part} l={l} at={at} close={close} />
          ))}
          {plain.length > 0 && (
            <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border text-sm text-fg">
              {plain.map((c, i) => (
                <li key={i} className="px-3 py-2">
                  {c}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
      {note !== undefined && <HandoffLines text={note} folded={changes.length > 0} />}
    </div>
  );
}

/** The handoff: folded when changes come first, else open; held to a few lines (whole Markdown, never cut mid-block) until Show all. */
export function HandoffLines({ text, folded }: { text: string | null; folded: boolean }) {
  if (text === null)
    return (
      <section className="flex flex-col gap-2">
        <Heading>Handoff</Heading>
        <p className="rounded-lg border border-border px-3 py-2 text-sm text-muted">None written</p>
      </section>
    );
  return <HandoffNote text={text} folded={folded} />;
}

function HandoffNote({ text, folded }: { text: string; folded: boolean }) {
  const [open, setOpen] = useState(!folded);
  const [all, setAll] = useState(false);
  const rows = text.split('\n');
  const hidden = rows.length - FIRST;
  return (
    <section className="flex flex-col gap-2">
      <Heading>Handoff</Heading>
      <div className="overflow-hidden rounded-lg border border-border">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className={`flex w-full items-center gap-2 bg-hover/40 px-3 py-1.5 text-left text-xs text-fg hover:bg-hover/70 ${open ? 'border-b border-border' : ''}`}
        >
          <ChevronRight size={14} className={`shrink-0 text-muted transition-transform ${open ? 'rotate-90' : ''}`} />
          <span className="flex-1">{open ? 'Hide handoff' : 'Show handoff'}</span>
          <span className="text-muted">{rows.length} lines</span>
        </button>
        {open && (
          <>
            <div className={`px-4 py-3 text-sm text-fg ${all || hidden <= 0 ? 'max-h-96 overflow-auto' : 'max-h-48 overflow-hidden'}`}>
              <Markdown text={text} />
            </div>
            {!all && hidden > 0 && (
              <button
                type="button"
                onClick={() => setAll(true)}
                className="w-full border-t border-border px-3 py-1.5 text-left text-xs text-accent hover:bg-hover/40"
              >
                Show all {rows.length} lines
              </button>
            )}
          </>
        )}
      </div>
    </section>
  );
}

export function Facts({ rows }: { rows: [string, string][] }) {
  return (
    <dl className="flex flex-col gap-1 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="flex gap-4">
          <dt className="w-16 shrink-0 text-muted">{k}</dt>
          <dd className="min-w-0 text-fg">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

/** What a compaction was: why and when first, then the note the agent carried over, read as Markdown. */
function Compaction({ at, why, handoff }: { at: string; why: string | null; handoff: string }) {
  return (
    <div className="flex flex-col gap-5 pb-1">
      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium text-muted">Summary</h3>
        <Facts
          rows={[
            ['Why', why ?? 'Not recorded'],
            ['When', new Date(at).toLocaleString()],
          ]}
        />
      </section>
      <section className="flex flex-col gap-2">
        <h3 className="text-xs font-medium text-muted">Handoff</h3>
        <div className="text-sm text-fg">
          <Markdown text={handoff} />
        </div>
      </section>
    </div>
  );
}
