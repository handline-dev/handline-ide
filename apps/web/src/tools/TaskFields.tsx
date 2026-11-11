// A task's fields that can be changed: assignee, priority and time box, each a label and its value; the value opens its choices.
import { assignGroups } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { useAgents, useTeam, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { ChevronDown, Flag, Timer, User } from 'lucide-react';
import { useState, type ComponentProps, type ReactElement, type ReactNode } from 'react';
import { Input } from '#src/ui/Field.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { ClockBar, LeftWords, WholeTime, useLeft } from '#src/tools/TaskClock.tsx';
import { Popover } from '#src/ui/Popover.tsx';
import { toast } from '#src/ui/toast.tsx';

const PRIORITIES = [1, 2, 3] as const;
const TIME_BOXES = ['30m', '1h', '2h', '4h', '8h'];

/** What can be changed, each a label and its value; the value opens its choices (a closed task shows them as words). */
export function Fields({ t, open }: { t: TaskDetail; open: boolean }) {
  const edit = useWrite('task.edit');
  const failed = (what: string) => (e: unknown) =>
    toast(`${what} did not change: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
  return (
    <dl className="grid items-start gap-x-6 gap-y-0.5 text-sm" style={{ gridTemplateColumns: 'auto 1fr' }}>
      <Assignee t={t} open={open} />
      <Field label="Priority" icon={<Flag size={14} />} value={t.priority ? `P${t.priority}` : 'None'} open={open}>
        {PRIORITIES.map((p) => (
          <MenuItem
            key={p}
            checked={t.priority === p}
            onClick={() => edit.mutate({ task: t.id, priority: p }, { onError: failed('Priority') })}
          >
            P{p}
          </MenuItem>
        ))}
      </Field>
      <TimeBox t={t} open={open} />
    </dl>
  );
}

const LIKELY = 5;

/** The assignee: a search over everyone, and before typing a short list of who is likely (who worked on it, near it in the team,
 * me, the owner now). A click or Enter hands it over; arrows move. */
function Assignee({ t, open }: { t: TaskDetail; open: boolean }) {
  const edit = useWrite('task.edit');
  const choose = (to: string) =>
    edit.mutate(
      { task: t.id, owner: to },
      { onError: (e) => toast(`Assignee did not change: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }) },
    );
  return (
    <>
      <Label icon={<User size={14} />}>Assignee</Label>
      <dd>
        {open ? (
          <AssignPicker t={t} trigger={<ValueButton>{t.to}</ValueButton>} onPick={choose} />
        ) : (
          <span className="text-fg">{t.to}</span>
        )}
      </dd>
    </>
  );
}

/** Who to give the task to: a search over everyone, and before typing a short list of who is likely (who worked on it, near it in the
 * team, me, the owner now). A click or Enter picks; arrows move. Picking the owner now picks nothing. */
export function AssignPicker({ t, trigger, onPick }: { t: TaskDetail; trigger: ReactElement; onPick: (to: string) => void }) {
  const me = useMe();
  const known = useAgents().data?.agents.map((a) => a.key) ?? [];
  const team = useTeam().data ?? { agents: [] };
  const [shown, setShown] = useState(false);
  const [find, setFind] = useState('');
  const [at, setAt] = useState(0);
  const groups = assignGroups(t, team, known, me);
  const likely = groups.filter((g) => g.key !== 'rest').flatMap((g) => g.names);
  const everyone = [...new Set([...groups.flatMap((g) => g.names), me, t.to])];
  const q = find.trim().toLowerCase();
  const names = q
    ? everyone.filter((n) => n.toLowerCase().includes(q) || (n === me && 'me'.includes(q))).slice(0, 8)
    : [...new Set([...likely.slice(0, LIKELY - 2), me, t.to])];
  const pick = names[Math.min(at, names.length - 1)];
  const choose = (to = pick) => {
    setShown(false);
    if (to && to !== t.to) onPick(to);
  };
  return (
    <Popover
      open={shown}
      onOpenChange={(o) => {
        setShown(o);
        setFind('');
        setAt(0);
      }}
      trigger={trigger}
    >
      <div className="flex w-64 flex-col gap-1">
        <Input
          autoFocus
          placeholder="Find someone"
          value={find}
          onChange={(e) => {
            setFind(e.target.value);
            setAt(0);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              choose();
            } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
              e.preventDefault();
              setAt((i) => (i + (e.key === 'ArrowDown' ? 1 : names.length - 1)) % Math.max(names.length, 1));
            }
          }}
        />
        <ul role="listbox" aria-label={q ? 'Matches' : 'Likely'} className="flex flex-col">
          {names.map((n) => (
            <li key={n} role="option" aria-selected={n === pick}>
              <button
                type="button"
                onMouseEnter={() => setAt(names.indexOf(n))}
                onClick={() => choose(n)}
                className={`flex h-7 w-full items-center justify-between gap-2 rounded-sm px-2 text-left ${n === pick ? 'bg-hover' : ''}`}
              >
                <span className="truncate">{n === me ? 'Me' : n}</span>
                {n === t.to && <span className="text-xs text-muted">Now</span>}
              </button>
            </li>
          ))}
        </ul>
        {!names.length && <p className="px-2 text-xs text-muted">No one matches.</p>}
        {!q && everyone.length > names.length && <p className="px-2 text-xs text-muted">Type to find more · {everyone.length} in all</p>}
      </div>
    </Popover>
  );
}

/** Like 45m, 1h30m, 2d: what the daemon takes as a duration. */
const DURATION = /^(\d+d)?(\d+h)?(\d+m)?$/;

/** The time box: what is left of it before its length, the length a usual one picked or any typed (Enter keeps it), and under them the
 * clock's bar. */
function TimeBox({ t, open }: { t: TaskDetail; open: boolean }) {
  const id = t.id;
  const value = t.time_box;
  const clockNow = useLeft(t);
  const edit = useWrite('task.edit');
  const [shown, setShown] = useState(false);
  const [typed, setTyped] = useState(value ?? '');
  const [custom, setCustom] = useState(false);
  const ok = !!typed && DURATION.test(typed.trim());
  const save = (b: string) => {
    setShown(false);
    if (b === value) return;
    edit.mutate(
      { task: id, time_box: b },
      { onError: (e) => toast(`Time box did not change: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }) },
    );
  };
  return (
    <>
      <Label icon={<Timer size={14} />}>Time box</Label>
      <dd className="flex min-w-0 flex-col">
        <span className="flex items-center gap-1 whitespace-nowrap">
          {clockNow && <LeftWords left={clockNow.left} />}
          {open ? (
            <Popover
              open={shown}
              onOpenChange={(o) => {
                setShown(o);
                if (o) setTyped(value ?? '');
                if (!o) setCustom(false);
              }}
              trigger={<ValueButton>{value ?? 'None'}</ValueButton>}
            >
              <form
                className="flex w-52 flex-col gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (ok) save(typed.trim());
                }}
              >
                <div className="flex flex-wrap gap-1">
                  {TIME_BOXES.map((b) => (
                    <button
                      key={b}
                      type="button"
                      onClick={() => save(b)}
                      className={`h-6 rounded-md px-2 text-xs ${b === value ? 'bg-selected text-fg' : 'text-muted hover:bg-hover/30'}`}
                    >
                      {b}
                    </button>
                  ))}
                  {/* Another length is typed: the field shows only when asked for, or when the length now is not one of the usual. */}
                  {!custom && (
                    <button
                      type="button"
                      onClick={() => setCustom(true)}
                      className={`h-6 rounded-md px-2 text-xs ${value && !TIME_BOXES.includes(value) ? 'bg-selected text-fg' : 'text-muted hover:bg-hover/30'}`}
                    >
                      {value && !TIME_BOXES.includes(value) ? value : 'Other…'}
                    </button>
                  )}
                </div>
                {custom && (
                  <Input
                    autoFocus
                    value={typed}
                    onChange={(e) => setTyped(e.target.value)}
                    placeholder="45m, 1h30m, 2d"
                    aria-invalid={!!typed && !ok}
                  />
                )}
              </form>
            </Popover>
          ) : (
            <span className="text-fg">{value ?? 'None'}</span>
          )}
        </span>
        {clockNow ? <ClockBar t={t} left={clockNow.left} total={clockNow.total} /> : <WholeTime task={id} />}
      </dd>
    </>
  );
}

/** A field's value as a button: a faint fill and a chevron on hover say it can be changed. */
function ValueButton(props: ComponentProps<'button'>) {
  return (
    <button type="button" {...props} className="group -ml-2 flex h-7 items-center gap-1 rounded-md px-2 text-fg hover:bg-hover/30">
      {props.children}
      <ChevronDown size={12} className="text-muted opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  );
}

/** A field's name, its icon before it. */
function Label({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <dt className="flex h-7 items-center gap-2 text-muted">
      {icon}
      {children}
    </dt>
  );
}

function Field({
  label,
  icon,
  value,
  open,
  children,
}: {
  label: string;
  icon: ReactNode;
  value: string;
  open: boolean;
  children: ReactNode;
}) {
  return (
    <>
      <Label icon={icon}>{label}</Label>
      <dd>{open ? <Menu trigger={<ValueButton>{value}</ValueButton>}>{children}</Menu> : <span className="text-fg">{value}</span>}</dd>
    </>
  );
}
