// The side panel the Town tab opens beside the city or an office: the decisions in the HQ or in an office's mailbox.
import type { ReactNode } from 'react';
import type { Activity } from '#src/layout/agentList.ts';
import type { Decision } from '#src/town/model.ts';

export const WORDS: Record<Activity, string> = {
  working: 'Working',
  waking: 'Starting',
  asleep: 'Asleep',
  you: 'Waiting for you',
  held: 'On hold',
  error: 'Stopped',
  resting: 'Idle',
};

/** A side panel beside the city or an office. */
export function Panel({ title, sub, onClose, children }: { title: string; sub: string; onClose: () => void; children: ReactNode }) {
  return (
    <aside className="flex w-80 shrink-0 flex-col border-l border-border bg-surface text-sm">
      <div className="flex h-9 shrink-0 items-center gap-2 px-3">
        <span className="truncate font-medium">{title}</span>
        <span className="truncate text-xs text-muted">{sub}</span>
        <span className="flex-1" />
        <button type="button" onClick={onClose} className="rounded-md px-1.5 text-muted hover:bg-hover hover:text-fg">
          Close
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
    </aside>
  );
}

/** Decisions as flat rows, under one heading or by workspace. */
export function Decisions({
  papers,
  heading,
  groups,
  onOpen,
}: {
  papers: Decision[];
  heading?: string;
  groups?: { key: string; name: string }[];
  onOpen: (d: Decision) => void;
}) {
  const sections = groups
    ? groups.map((g) => ({ name: g.name, rows: papers.filter((p) => (p.house ?? '') === g.key) }))
    : [{ name: heading ?? '', rows: papers }];
  if (papers.length === 0) return <p className="px-3 py-2 text-muted">Nothing waits for you.</p>;
  return sections
    .filter((s) => s.rows.length)
    .map((s) => (
      <section key={s.name}>
        <h3 className="sticky top-0 bg-surface px-3 py-1 text-xs text-muted">{s.name}</h3>
        {s.rows.map((d) => (
          <button key={d.id} type="button" onClick={() => onOpen(d)} className="flex w-full flex-col px-3 py-1.5 text-left hover:bg-hover">
            <span className="truncate">{d.title}</span>
            <span className="truncate text-xs text-muted">{[d.agent, d.task ? `T${d.task}` : null].filter(Boolean).join(' · ')}</span>
          </button>
        ))}
      </section>
    ));
}
