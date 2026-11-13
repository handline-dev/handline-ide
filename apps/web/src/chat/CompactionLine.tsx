import { motion } from 'framer-motion';
import { FoldVertical, RotateCw } from 'lucide-react';
import { useState } from 'react';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';
import { Markdown } from '#src/ui/Markdown.tsx';
import type { LineChange } from '@handline/client';
import { ModalFrame, openModal } from '#src/ui/modal.tsx';
import { textOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { write } from '#src/persist/kv.ts';
import { textKey } from '#src/tools/TextTab.tsx';

/** Why a conversation began or restarted, as the line says it; a word the daemon adds later shows as it is. */
const causes: Record<string, string> = {
  launch: 'first start',
  start: 'new work arrived',
  wake: 'cache expired',
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
  resumed,
  changed,
  changed_lines: lines = [],
}: {
  at: string;
  cause?: string;
  handoff: string | null;
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
        {clock(at)}
      </motion.time>
    </>
  );
  const changes = changesOf(changed);
  const open = () =>
    void openModal((close) =>
      resumed ? (
        <ModalFrame title="Restarted" onClose={() => close()}>
          <Restart at={at} changes={changes} lines={lines} close={() => close()} />
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
      {(resumed ? changes.length : handoff) ? (
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

/** One part's removed (−) and added (+) lines, the first few. Show all opens the part's whole text in a tab beside when the daemon
 * kept it, else unfolds the rest of the lines here. */
function PartLines({ l, at, close }: { l: LineChange; at: string; close: () => void }) {
  const [all, setAll] = useState(false);
  const openTab = useOpenTab();
  const whole = () => {
    const opens = textOf(at, l.part);
    write(textKey(opens.slice(5)), l.text);
    close();
    openTab(opens, 'side');
  };
  const rows = [...l.removed.map((t) => ({ t, add: false })), ...l.added.map((t) => ({ t, add: true }))];
  const shown = all ? rows : rows.slice(0, FIRST);
  const hidden = rows.length - shown.length;
  return (
    <div className="flex flex-col gap-1">
      <div className="text-sm font-medium text-fg">{l.part}</div>
      <pre className="max-h-96 overflow-auto rounded-md bg-hover/40 px-3 py-2 font-mono text-xs leading-5 whitespace-pre-wrap">
        {shown.map((r, i) => (
          <div key={i} className={r.add ? 'text-success' : 'text-danger'}>
            {r.add ? '+' : '−'} {r.t}
          </div>
        ))}
        {!!l.more && hidden === 0 && <div className="text-muted">and {l.more} more lines, too many to keep</div>}
      </pre>
      {l.text != null ? (
        <button type="button" onClick={whole} className="self-start rounded px-1 text-xs text-accent hover:bg-hover">
          Show all
        </button>
      ) : (
        hidden > 0 && (
          <button type="button" onClick={() => setAll(true)} className="self-start rounded px-1 text-xs text-accent hover:bg-hover">
            Show all {rows.length + (l.more ?? 0)} lines
          </button>
        )
      )}
    </div>
  );
}

/** What a restart changed, one change a line, under when it was. */
function Restart({ at, changes, lines, close }: { at: string; changes: string[]; lines: LineChange[]; close: () => void }) {
  const shown = new Set(lines.map((l) => l.part));
  return (
    <div className="flex flex-col gap-5 pb-1">
      <Facts rows={[['When', new Date(at).toLocaleString()]]} />
      <section className="flex flex-col gap-3">
        <h3 className="text-xs font-medium text-muted">What changed</h3>
        {lines.map((l) => (
          <PartLines key={l.part} l={l} at={at} close={close} />
        ))}
        {/* A change the daemon kept no lines for (a model, or an older daemon) stays a plain line. */}
        <ul className="flex list-disc flex-col gap-1 pl-5 text-sm text-fg">
          {changes
            .filter((c) => !shown.has(c.replace(/ changed$/, '')))
            .map((c) => (
              <li key={c}>{c}</li>
            ))}
        </ul>
      </section>
    </div>
  );
}

function Facts({ rows }: { rows: [string, string][] }) {
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
