// What a right-click on a row of Requests offers, and what can be done to several picked rows at once. Only what is easy to undo
// goes at once (seen, later, give back); a cancel, of one row or several, is asked for one reason and confirmed.
import { bulkOf, foundersTurn, laterChoices, laterKey, type Picked } from '@handline/app-core';
import { useTaskActions, useTasks, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { Ban, Clock, Copy, Eye, FolderOpen, MessageSquare, Undo2 } from 'lucide-react';
import { createContext, useContext, useState, type ReactNode } from 'react';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { copy } from '#src/ui/copy.ts';
import { Button } from '#src/ui/Button.tsx';
import { ContextMenu, Menu, MenuItem, MenuSeparator, SubMenu } from '#src/ui/Menu.tsx';
import { confirm, prompt } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

type Pick = {
  picked: Map<string, Picked>;
  toggle: (x: Picked) => void;
  pickAll: (rows: Picked[]) => void;
  clear: () => void;
  /** Drops the picked rows that are no longer listed. */
  keep: (keys: ReadonlySet<string>) => void;
  /** Rows whose write is on its way: they dim until it lands. */
  busy: ReadonlySet<string>;
  setBusy: (keys: string[], on: boolean) => void;
};
const PickCtx = createContext<Pick | null>(null);

/** Holds the picked rows of one Requests view; the bell's preview has none, so its rows only open. */
export function PickProvider({ children }: { children: ReactNode }) {
  const [picked, setPicked] = useState(new Map<string, Picked>()); // not kept: a selection lasts while it is acted on
  const [busy, setBusyKeys] = useState(new Set<string>());
  const value: Pick = {
    picked,
    busy,
    toggle: (x) =>
      setPicked((m) => {
        const n = new Map(m);
        if (n.has(x.key)) n.delete(x.key);
        else n.set(x.key, x);
        return n;
      }),
    pickAll: (rows) => setPicked(new Map(rows.map((x) => [x.key, x]))),
    clear: () => setPicked(new Map()),
    keep: (keys) => setPicked((m) => ([...m.keys()].every((k) => keys.has(k)) ? m : new Map([...m].filter(([k]) => keys.has(k))))),
    setBusy: (keys, on) =>
      setBusyKeys((s) => {
        const n = new Set(s);
        for (const k of keys) {
          if (on) n.add(k);
          else n.delete(k);
        }
        return n;
      }),
  };
  return <PickCtx.Provider value={value}>{children}</PickCtx.Provider>;
}
export const usePick = () => useContext(PickCtx);

/** The tasks whose top turn the founder holds: only there does Give back pass a turn, and not end an agent's. */
function useMyTurns(): ReadonlySet<number> {
  const me = useMe();
  return new Set((useTasks({}).data?.items ?? []).filter((t) => foundersTurn(t, me)).map((t) => t.id));
}

/** The writes behind the menu: each runs on every row given, dims those rows until all land, and says in a toast when one fails. */
export function useRowActions() {
  const pick = usePick();
  const pop = useWrite('task.turn.pop');
  const seen = useWrite('task.seen');
  const prefs = useWrite('prefs.set');
  const { verbs } = useTaskActions();
  const all = async (what: string, rows: { key: string; run: () => Promise<unknown> }[]) => {
    const keys = rows.map((r) => r.key);
    pick?.setBusy(keys, true);
    const failed = (await Promise.allSettled(rows.map((r) => r.run()))).filter((r) => r.status === 'rejected').length;
    pick?.setBusy(keys, false);
    if (failed) toast(`${failed} of ${rows.length} could not be ${what}.`, { tone: 'warning' });
  };
  type On = { key: string; task: number };
  return {
    giveBack: (on: On[]) =>
      all(
        'given back',
        on.map((x) => ({ key: x.key, run: () => pop.mutateAsync({ task: x.task, outcome: 'pass' }) })),
      ),
    seen: (on: On[]) =>
      all(
        'marked seen',
        on.map((x) => ({ key: x.key, run: () => seen.mutateAsync({ task: x.task }) })),
      ),
    later: (on: { key: string; later: string }[], until: string) =>
      all(
        'put off',
        on.map((x) => ({ key: x.key, run: () => prefs.mutateAsync({ changes: { nav: { later: { [x.later]: { until } } } } }) })),
      ),
    /** One reason and one confirm for all the rows given; says whether they went. */
    cancel: async (on: (On & { title: string })[]): Promise<boolean> => {
      const one = on.length === 1 ? on[0] : null;
      const reason = (
        await prompt({ title: one ? `Cancel T${one.task}?` : `Cancel ${on.length} requests?`, placeholder: 'Why, in one line' })
      )?.trim();
      if (!reason) return false;
      const sure = await confirm({
        title: one ? `Cancel “${one.title}”?` : `Cancel these ${on.length} requests?`,
        message: one ? (
          reason
        ) : (
          <>
            {reason}
            <ul className="mt-2 max-h-40 list-disc overflow-y-auto pl-4">
              {on.map((x) => (
                <li key={x.task}>{x.title}</li>
              ))}
            </ul>
          </>
        ),
        confirmText: one ? 'Cancel it' : 'Cancel them',
        cancelText: one ? 'Keep it' : 'Keep them',
        danger: true,
      });
      if (!sure) return false;
      await all(
        'cancelled',
        on.map((x) => ({ key: x.key, run: () => verbs.cancel(x.task, reason) })),
      );
      return true;
    },
  };
}

function LaterItems({ onPick }: { onPick: (until: string) => void }) {
  return laterChoices(Date.now()).map((c) => (
    <MenuItem key={c.until} onClick={() => onPick(c.until)}>
      {c.label}
    </MenuItem>
  ));
}

/** What can be done to all the picked rows, the same in the floating bar (as buttons) and a picked row's right-click (as items). */
function useBulk() {
  const pick = usePick();
  const act = useRowActions();
  const b = bulkOf([...(pick?.picked.values() ?? [])], useMyTurns());
  const then = (run: Promise<unknown>) => void run.then(() => pick?.clear());
  return {
    seen: b.seen.length ? { label: `Mark ${b.seen.length} seen`, run: () => then(act.seen(b.seen)) } : null,
    later: b.later.length ? { label: `Put ${b.later.length} off`, run: (until: string) => then(act.later(b.later, until)) } : null,
    giveBack: b.giveBack.length ? { label: `Give ${b.giveBack.length} back`, run: () => then(act.giveBack(b.giveBack)) } : null,
    // Backing out of its questions keeps the selection.
    cancel: b.cancel.length
      ? { label: `Cancel ${b.cancel.length}`, run: () => void act.cancel(b.cancel).then((went) => went && pick?.clear()) }
      : null,
  };
}

export function BulkItems() {
  const pick = usePick();
  const b = useBulk();
  if (!pick) return null;
  return (
    <>
      {b.seen && (
        <MenuItem icon={<Eye size={14} />} onClick={b.seen.run}>
          {b.seen.label}
        </MenuItem>
      )}
      {b.later && (
        <SubMenu label={b.later.label} icon={<Clock size={14} />}>
          <LaterItems onPick={b.later.run} />
        </SubMenu>
      )}
      {b.giveBack && (
        <MenuItem icon={<Undo2 size={14} />} onClick={b.giveBack.run}>
          {b.giveBack.label}
        </MenuItem>
      )}
      {b.cancel && (
        <MenuItem icon={<Ban size={14} />} danger onClick={b.cancel.run}>
          {`${b.cancel.label}…`}
        </MenuItem>
      )}
      <MenuSeparator />
      <MenuItem onClick={() => pick.clear()}>Clear the selection</MenuItem>
    </>
  );
}

export function BulkButtons() {
  const b = useBulk();
  if (!b.seen && !b.later && !b.giveBack && !b.cancel)
    return <span className="px-2 whitespace-nowrap text-muted">Nothing to do to all of these</span>;
  return (
    <>
      {b.seen && (
        <Button variant="ghost" onClick={b.seen.run}>
          <Eye size={14} />
          {b.seen.label}
        </Button>
      )}
      {b.later && (
        <Menu
          side="top"
          trigger={
            <Button variant="ghost">
              <Clock size={14} />
              {b.later.label}
            </Button>
          }
        >
          <LaterItems onPick={b.later.run} />
        </Menu>
      )}
      {b.giveBack && (
        <Button variant="ghost" onClick={b.giveBack.run}>
          <Undo2 size={14} />
          {b.giveBack.label}
        </Button>
      )}
      {b.cancel && (
        <Button variant="ghost" onClick={b.cancel.run}>
          <Ban size={14} />
          {`${b.cancel.label}…`}
        </Button>
      )}
    </>
  );
}

/** A right-click on a row: its own actions, or, on a row among several picked, what can be done to all of them. */
export function RowMenu({ row, children }: { row: Picked; children: ReactNode }) {
  const pick = usePick();
  const openTab = useOpenTab();
  const act = useRowActions();
  const mine = useMyTurns();
  const many = !!pick && pick.picked.size > 1 && pick.picked.has(row.key);
  const { p, r, key } = row;
  const task = p?.task ?? r?.task ?? null;
  const later = p ? laterKey(p) : null;
  const who = p?.agent ?? r?.now ?? r?.agent;
  const items = many ? (
    <BulkItems />
  ) : (
    <>
      {task !== null && (
        <MenuItem icon={<FolderOpen size={14} />} onClick={() => openTab(taskOf(task), 'here')}>
          {`Open T${task}`}
        </MenuItem>
      )}
      {who && (
        <MenuItem icon={<MessageSquare size={14} />} onClick={() => openTab(chatOf(who), 'here')}>
          {`Open ${who}'s chat`}
        </MenuItem>
      )}
      {p && task !== null && mine.has(task) && (
        <MenuItem icon={<Undo2 size={14} />} onClick={() => void act.giveBack([{ key, task }])}>
          Give back
        </MenuItem>
      )}
      {later && (
        <SubMenu label="Later" icon={<Clock size={14} />}>
          <LaterItems onPick={(until) => void act.later([{ key, later }], until)} />
        </SubMenu>
      )}
      {r && task !== null && r.ended && !r.ended.seen && (
        <MenuItem icon={<Eye size={14} />} onClick={() => void act.seen([{ key, task }])}>
          Mark seen
        </MenuItem>
      )}
      {r && task !== null && !r.ended && (
        <MenuItem icon={<Ban size={14} />} danger onClick={() => void act.cancel([{ key, task, title: r.title }])}>
          Cancel…
        </MenuItem>
      )}
      {task !== null && (
        <>
          <MenuSeparator />
          <MenuItem icon={<Copy size={14} />} onClick={() => void copy(`T${task}`, 'the task number', { say: true })}>
            {`Copy T${task}`}
          </MenuItem>
        </>
      )}
    </>
  );
  return <ContextMenu items={items}>{children}</ContextMenu>;
}
