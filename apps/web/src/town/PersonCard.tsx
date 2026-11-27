// The card that opens while the pointer rests on a person or its desk in an office (Targets.tsx): who it is and in which team, its role,
// its state in words with the app's own mark, what it does now and how far along, what waits behind that, and what it runs on.
import { Hooks } from '@handline/app-data';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { Face } from '#src/town/face.tsx';
import { kindOf, stepsOf, waitingOf } from '#src/town/marks.ts';
import { WORDS, type Person } from '#src/town/model.ts';

/** How far along `task` is and what waits behind it, in one quiet line; nothing when there is neither. */
function Progress({ task, waiting }: { task: number | null; waiting: string | null }) {
  const steps = stepsOf(Hooks.useTaskGet({ task: task ?? 0 }, { enabled: task !== null }).data?.todo);
  const parts = [steps && `${steps.text} steps`, waiting].filter(Boolean);
  return parts.length ? <p className="text-xs text-muted tabular-nums">{parts.join(' · ')}</p> : null;
}

/** `row`: its figure on the people sheet, for its face. */
export function PersonCard({ person: p, row }: { person: Person; row: number | undefined }) {
  const doing = p.doing?.trim();
  // The reason only for one on hold or stopped, and only when it says more than the state's own word.
  const stuck = p.activity === 'held' || p.activity === 'error';
  const why = stuck && p.why?.trim() !== WORDS[p.activity] ? p.why?.trim() : null;
  const kind = kindOf(p);
  return (
    <div className="flex w-64 flex-col gap-2 p-2 text-sm">
      <div className="flex items-center gap-2.5">
        {row !== undefined && <Face row={row} size={40} whole />}
        <div className="flex min-w-0 flex-col">
          <span className="truncate font-medium">{p.name}</span>
          {p.team && <span className="truncate text-xs text-muted">{p.team}</span>}
        </div>
      </div>
      {p.role && <p className="line-clamp-2 text-xs text-muted">{p.role}</p>}
      <div className="flex items-start gap-1.5">
        <span className="flex h-5 shrink-0 items-center">
          <ActivityIcon act={p.activity} />
        </span>
        <span className="min-w-0">
          {WORDS[p.activity]}
          {why && <span className="text-muted"> · {why}</span>}
        </span>
      </div>
      {doing && <p className="line-clamp-2">{doing}</p>}
      <Progress task={p.task ?? null} waiting={waitingOf(p)} />
      {kind && <p className="text-xs text-muted">{kind}</p>}
      <p className="border-t border-border pt-1.5 text-xs text-muted">Click to chat · right-click for more</p>
    </div>
  );
}
