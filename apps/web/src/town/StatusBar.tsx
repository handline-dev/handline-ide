// The town's top strip, beside its name: how many of the whole town are at work, waiting for the founder, stopped and on hold. Each count
// opens the list of those people, with their team and what they are doing; a click on one goes to its office and brings up its chat
// (goto.ts). A count of none is not shown, but the people at work always are. Last, the work finished today: each task opens in the
// town's windows.
import { Hooks, useAgents, useNameOf, useTasks } from '@handline/app-data';
import { CircleCheck } from 'lucide-react';
import { useMemo, useState, type ReactNode } from 'react';
import { useOpenable } from '#src/layout/openable.tsx';
import { Ago } from '#src/ui/Ago.tsx';
import { useNow } from '#src/format/time.ts';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { Popover } from '#src/ui/Popover.tsx';
import { Face } from '#src/town/face.tsx';
import { townGo } from '#src/town/goto.ts';
import { townOf, WORDS } from '#src/town/model.ts';
import { figuresOf } from '#src/town/people.ts';
import { GROUPS, GROUP_WORDS, doneToday, rollCallOf, type Called, type Finished, type Group } from '#src/town/rollcall.ts';

const NONE: never[] = [];

export function TownStatus() {
  const workspaces = Hooks.useWorkspacesList().data?.workspaces;
  const agents = useAgents().data?.agents;
  const now = useNow(60_000);
  const town = useMemo(() => townOf(workspaces ?? NONE, agents ?? NONE, NONE, now), [workspaces, agents, now]);
  const call = useMemo(() => rollCallOf(town.houses), [town]);
  // Each person's face, as its office draws it.
  const faces = useMemo(() => new Map(town.houses.flatMap((h) => [...figuresOf(h.people)])), [town]);
  if (!agents) return null;
  return (
    <div className="app-no-drag flex min-w-0 items-center gap-1 overflow-hidden">
      {GROUPS.filter((g) => g === 'working' || call[g].length > 0).map((g) => (
        <Count key={g} group={g} people={call[g]} faces={faces} />
      ))}
      <Done now={now} />
    </div>
  );
}

const CHIP =
  'flex shrink-0 items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs text-muted hover:bg-hover hover:text-fg disabled:pointer-events-none data-[popup-open]:bg-hover data-[popup-open]:text-fg';
const ROW = 'flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left hover:bg-hover';

/** A count of the strip: its mark, its number and its words ("3 working"), the words left out on a phone's width; a click lists `rows`. */
function Chip({ mark, n, words, children }: { mark: ReactNode; n: number; words: string; children: (close: () => void) => ReactNode }) {
  const [open, setOpen] = useState(false);
  const said = `${n} ${words}`;
  const chip = (
    <button type="button" disabled={!n} title={said} aria-label={said} className={CHIP}>
      {mark}
      <span>
        {n}
        <span className="max-sm:hidden"> {words}</span>
      </span>
    </button>
  );
  return (
    <Popover trigger={chip} open={open} onOpenChange={setOpen}>
      <ul className="-m-1.5 flex max-h-96 w-72 flex-col overflow-y-auto" aria-label={said}>
        {children(() => setOpen(false))}
      </ul>
    </Popover>
  );
}

function Count({ group, people, faces }: { group: Group; people: Called[]; faces: ReadonlyMap<string, number> }) {
  const n = people.length;
  // Nobody at work: a still ring, not the turning arc.
  const mark = <ActivityIcon act={group === 'working' && !n ? 'resting' : group} />;
  return (
    <Chip mark={mark} n={n} words={GROUP_WORDS[group]}>
      {(close) =>
        people.map(({ person, house }) => (
          <li key={person.key}>
            <button
              type="button"
              className={ROW}
              onClick={() => {
                close();
                townGo(house.key, person);
              }}
            >
              <Face row={faces.get(person.key) ?? 0} size={24} />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="flex items-baseline gap-1.5">
                  <span className="truncate font-medium">{person.name}</span>
                  <span className="truncate text-xs text-muted">{house.name}</span>
                </span>
                <span className="truncate text-xs text-muted">{person.doing ?? WORDS[person.activity]}</span>
              </span>
            </button>
          </li>
        ))
      }
    </Chip>
  );
}

/** The tasks done today, newest first; shown once there is one. */
function Done({ now }: { now: number }) {
  // The most a page holds: a day of more than that shows that many.
  const closed = useTasks({ state: 'done', order: 'closed', limit: 200 }).data?.items;
  const done = useMemo(() => doneToday(closed ?? NONE, now), [closed, now]);
  if (!done.length) return null;
  return (
    <Chip mark={<CircleCheck size={14} className="text-success" />} n={done.length} words="done today">
      {(close) => done.map((t) => <DoneRow key={t.id} task={t} onPick={close} />)}
    </Chip>
  );
}

function DoneRow({ task, onPick }: { task: Finished; onPick: () => void }) {
  const nameOf = useNameOf();
  const open = useOpenable(`task:${task.id}`);
  return (
    <li>
      <button
        type="button"
        {...open}
        className={ROW}
        onClick={(e) => {
          onPick();
          open.onClick?.(e);
        }}
      >
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate font-medium">{task.title}</span>
          <span className="truncate text-xs text-muted">
            {nameOf(task.to)}
            {task.done && (
              <>
                {' · '}
                <Ago at={task.done} />
              </>
            )}
          </span>
        </span>
      </button>
    </li>
  );
}
