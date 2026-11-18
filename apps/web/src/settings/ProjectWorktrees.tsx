// A project's worktree folders (desks): how many it keeps, Auto or Manual, and each folder, who uses it, its size and its pin.
import { bytesText } from '@handline/app-core';
import { useDesks, useNameOf, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Methods, Repo } from '@handline/client';
import { Pin } from 'lucide-react';
import { ago } from '#src/format/time.ts';
import { Setting } from '#src/settings/Meter.tsx';
import { NumberStepper } from '#src/ui/NumberStepper.tsx';
import { Switch } from '#src/usage/UsageViews.tsx';

type Desk = Methods['desks.list']['result']['desks'][number];
const MODES = ['auto', 'manual'] as const;
const num = (d: Desk) => Number(/(\d+)$/.exec(d.name)?.[1] ?? Number.MAX_SAFE_INTEGER);

export function ProjectWorktrees({ repo }: { repo: Repo }) {
  const read = useDesks(repo.path);
  const keep = useWrite('desks.keep');
  const pin = useWrite('desk.pin');
  const me = useMe();
  const nameOf = useNameOf();
  const desks = [...(read.data?.desks ?? [])].sort((a, b) => num(a) - num(b) || a.name.localeCompare(b.name));
  const kept = read.data?.kept ?? null;
  const total = desks.reduce((n, d) => n + (d.size_bytes ?? 0), 0);
  const who = (d: Desk) => {
    if (d.holder)
      return (
        <span className="flex items-center gap-1.5 text-fg">
          <span className="size-1.5 shrink-0 rounded-full bg-success" aria-label="in use" />
          {nameOf(d.holder)}
          {d.task != null && <span className="text-muted">· T{d.task}</span>}
        </span>
      );
    if (d.pinned_to && d.pinned_to !== 'removing')
      return (
        <span className="text-fg">
          {d.pinned_to === 'founder' ? 'You' : nameOf(d.pinned_to)} <span className="text-muted">· pinned</span>
        </span>
      );
    const last = d.last_agent
      ? `last ${d.last_agent === 'founder' ? 'you' : nameOf(d.last_agent)}, ${ago(new Date(d.last_used * 1000).toISOString())} ago`
      : null;
    return (
      <span className="text-muted">
        Free{last && ` · ${last}`}
        {d.removed_tonight && kept?.value != null && ` · over ${kept.value}, removed tonight`}
      </span>
    );
  };
  return (
    <section className="flex flex-col gap-3 border-t border-border pt-3 pb-1 pl-6" aria-label={`Worktrees of ${repo.name}`}>
      <div>
        <h3 className="text-sm font-medium text-fg">Worktrees</h3>
        <p className="text-xs text-muted">
          Agents take a free folder when they start work and give it back when their session closes. Their branches stay.
        </p>
      </div>
      {kept && (
        <Setting
          title="Folders kept"
          note={
            kept.mode === 'auto'
              ? `Auto: the most agents working at once on a day, averaged over 7 days, plus one${kept.value == null ? ' (no week of work yet, so nothing is removed)' : ''}. Folders over this number are removed after 3 days unused, never a pinned one.`
              : 'Folders over this number are removed after 3 days unused, never a pinned one.'
          }
        >
          <div className={`flex items-center gap-2 ${keep.isPending ? 'opacity-50' : ''}`}>
            <Switch
              label="Folders kept"
              value={kept.mode}
              options={MODES}
              word={(m) => (m === 'auto' ? 'Auto' : 'Manual')}
              onChange={(m) => {
                if (m === kept.mode) return;
                keep.mutate(
                  m === 'auto'
                    ? { repo: repo.path, mode: 'auto' }
                    : { repo: repo.path, mode: 'manual', value: Math.max(1, kept.value ?? kept.auto_value ?? desks.length) },
                );
              }}
            />
            {kept.mode === 'manual' ? (
              <NumberStepper
                value={kept.value ?? 1}
                min={1}
                max={50}
                label="Folders kept"
                disabled={keep.isPending}
                onCommit={(n) => keep.mutateAsync({ repo: repo.path, mode: 'manual', value: n })}
              />
            ) : (
              <span className="w-10 text-right text-sm text-fg tabular-nums">{kept.value ?? '–'}</span>
            )}
          </div>
        </Setting>
      )}
      {read.isError && <p className="text-xs text-danger">Could not read the folders: {String(read.error)}</p>}
      {read.data && !desks.length && <p className="text-xs text-muted">No folders yet: the first agent to start work here makes one.</p>}
      {desks.length > 0 && (
        <div className="max-h-96 overflow-auto rounded-md border border-border">
          <div className="sticky top-0 border-b border-border bg-surface px-3 py-1.5 text-xs font-medium tracking-wide text-muted uppercase">
            {desks.length} {desks.length === 1 ? 'folder' : 'folders'} · {bytesText(total)}
          </div>
          <ul>
            {desks.map((d) => {
              const pinned = !!d.pinned_to && d.pinned_to !== 'removing';
              return (
                <li
                  key={d.path}
                  className="flex items-center gap-3 border-b border-border px-3 py-2 text-sm last:border-b-0"
                  title={d.path}
                >
                  <span className="w-20 shrink-0 font-mono text-fg">{d.name}</span>
                  <span className="min-w-0 flex-1 truncate">{who(d)}</span>
                  <span className="shrink-0 text-xs text-muted tabular-nums">{d.size_bytes == null ? '' : bytesText(d.size_bytes)}</span>
                  <button
                    type="button"
                    aria-pressed={pinned}
                    aria-label={pinned ? `Unpin ${d.name}` : `Pin ${d.name} for you`}
                    title={pinned ? 'Pinned: no agent takes it and it is never removed' : 'Pin: keep it for you'}
                    disabled={pin.isPending || d.pinned_to === 'removing'}
                    onClick={() => pin.mutate(pinned ? { desk: d.path } : { desk: d.path, to: me })}
                    className={`shrink-0 rounded p-1 hover:bg-hover ${pinned ? 'text-accent' : 'text-muted'}`}
                  >
                    <Pin size={14} />
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </section>
  );
}
