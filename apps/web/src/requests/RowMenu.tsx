// What a right-click on a row of Requests offers, and what can be done to several picked rows at once. Only what is easy to undo
// goes at once (seen, later, give back, done); a discard, of one row or several, is asked for one reason and confirmed.
import { bulkNames, bulkOf, foundersTurn, laterChoices, laterKey, pickGroup, pickRange, type Picked } from '@handline/app-core';
import { useNameOf, useProposals, useTaskActions, useTasks, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { Ban, Check, Clock, Eye, MessageSquare, Undo2, X } from 'lucide-react';
import { createContext, useContext, useEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react';
import { chatOf } from '#src/layout/kinds.tsx';
import { OpenItems } from '#src/layout/openable.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { Button } from '#src/ui/Button.tsx';
import { ContextMenu, Menu, MenuItem, MenuSeparator, SubMenu } from '#src/ui/Menu.tsx';
import { confirm, prompt } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

type Pick = {
  picked: Map<string, Picked>;
  toggle: (x: Picked) => void;
  /** A Shift-click on `x`'s box: the shown rows from the last one clicked to `x` take that row's state (pickRange). */
  range: (x: Picked) => void;
  /** Each listed row says it is there, so a range can find the rows between two. */
  listed: Map<string, Picked>;
  /** Picks, or with `on` false unpicks, every row given: a section's box. */
  pickAll: (rows: Picked[], on: boolean) => void;
  /** The rows an action of the bar would act on, while the pointer is on it: they stand out. */
  hint: ReadonlySet<string> | null;
  setHint: (keys: ReadonlySet<string> | null) => void;
  clear: () => void;
  /** Drops the picked rows that are no longer listed. */
  keep: (keys: ReadonlySet<string>) => void;
  /** Rows whose write is on its way: they dim until it lands. */
  busy: ReadonlySet<string>;
  setBusy: (keys: string[], on: boolean) => void;
};
const PickCtx = createContext<Pick | null>(null);

/** Rows that are not picked with the rest (another cluster's): `off` leaves the pick on. */
export function NoPick({ off, children }: { off?: boolean; children: ReactNode }) {
  return off ? children : <PickCtx.Provider value={null}>{children}</PickCtx.Provider>;
}

/** Holds the picked rows of one Requests view; the bell's preview has none, so its rows only open. */
export function PickProvider({ children }: { children: ReactNode }) {
  const [picked, setPicked] = useState(new Map<string, Picked>()); // not kept: a selection lasts while it is acted on
  const [busy, setBusyKeys] = useState(new Set<string>());
  const [hint, setHint] = useState<ReadonlySet<string> | null>(null);
  const list = useRef<HTMLDivElement>(null);
  const [listed] = useState(() => new Map<string, Picked>());
  const anchor = useRef<string | null>(null);
  const value: Pick = {
    picked,
    busy,
    hint,
    listed,
    range: (x) => {
      const keys = [...(list.current?.querySelectorAll<HTMLElement>('[data-pick-key]') ?? [])].map((el) => el.dataset.pickKey!);
      const { keys: span, on } = pickRange(keys, anchor.current, x.key, (k) => picked.has(k));
      anchor.current = x.key;
      value.pickAll(
        span.filter((k) => !busy.has(k)).flatMap((k) => listed.get(k) ?? []),
        on,
      );
    },
    toggle: (x) => {
      anchor.current = x.key;
      setPicked((m) => {
        const first = m.values().next().value;
        if (first && pickGroup(first) !== pickGroup(x)) return m; // the other kind is dimmed, not picked
        const n = new Map(m);
        if (!n.delete(x.key)) n.set(x.key, x);
        return n;
      });
    },
    pickAll: (rows, on) =>
      setPicked((m) => {
        const n = new Map(m);
        // One kind at a time: the kind already picked, else the first row's.
        const kind = m.values().next().value ?? rows[0];
        for (const x of kind ? rows.filter((r) => pickGroup(r) === pickGroup(kind)) : []) {
          if (on) n.set(x.key, x);
          else n.delete(x.key);
        }
        return n;
      }),
    clear: () => setPicked(new Map()),
    setHint,
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
  return (
    <PickCtx.Provider value={value}>
      <div ref={list} className="contents">
        {children}
      </div>
    </PickCtx.Provider>
  );
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
  /** Says which rows failed: the picks keep them. With `again`, the refused are tried again while some still go through, so a parent
   * picked with its sub-tasks closes after them. */
  const all = async (what: string, rows: { keys: string[]; run: () => Promise<unknown> }[], again = false): Promise<string[]> => {
    const keys = rows.flatMap((r) => r.keys);
    pick?.setBusy(keys, true);
    let settled = await Promise.allSettled(rows.map((r) => r.run()));
    const refused = () => settled.filter((s) => s.status === 'rejected').length;
    for (let left = refused(), before = rows.length; again && left > 0 && left < before; before = left, left = refused()) {
      const retried = await Promise.allSettled(rows.map((r, i) => (settled[i]?.status === 'rejected' ? r.run() : undefined)));
      settled = settled.map((s, i) => (s.status === 'rejected' ? retried[i] : s));
    }
    pick?.setBusy(keys, false);
    const failed = rows.flatMap((r, i) => {
      const s = settled[i];
      return s?.status === 'rejected' ? [{ r, why: s.reason as unknown }] : [];
    });
    if (failed.length === 1) sayRefusal(failed[0].why);
    else if (failed.length) toast(`${failed.length} of ${rows.length} could not be ${what}.`, { tone: 'warning' });
    return failed.flatMap((f) => f.r.keys);
  };
  type On = { keys: string[]; task: number };
  const decide = useWrite('proposal.decide');
  const listed = useProposals().data?.proposals ?? [];
  return {
    /** Each proposal answered; an Allow names the subject proposal.list showed, as the row's own Allow does. */
    answer: (on: { keys: string[]; proposal: number }[], decision: 'approve' | 'reject') =>
      all(
        decision === 'approve' ? 'allowed' : 'denied',
        on.map((x) => {
          const subject = listed.find((l) => Number(l.id) === x.proposal)?.subject;
          return {
            keys: x.keys,
            run: () => decide.mutateAsync({ proposal: x.proposal, decision, ...(decision === 'approve' && subject ? { subject } : {}) }),
          };
        }),
      ),
    giveBack: (on: On[]) =>
      all(
        'given back',
        on.map((x) => ({ keys: x.keys, run: () => pop.mutateAsync({ task: x.task, outcome: 'pass' }) })),
      ),
    seen: (on: On[]) =>
      all(
        'marked seen',
        on.map((x) => ({ keys: x.keys, run: () => seen.mutateAsync({ task: x.task }) })),
      ),
    later: (on: { keys: string[]; later: string }[], until: string) =>
      all(
        'put off',
        on.map((x) => ({ keys: x.keys, run: () => prefs.mutateAsync({ changes: { nav: { later: { [x.later]: { until } } } } }) })),
      ),
    done: (on: On[]) =>
      all(
        'marked done',
        on.map((x) => ({ keys: x.keys, run: () => verbs.done(x.task, '') })),
        true,
      ),
    /** One reason and one confirm for all the rows given; null when the founder backs out, else the rows that failed. */
    discard: async (on: (On & { title: string })[]): Promise<string[] | null> => {
      const one = on.length === 1 ? on[0] : null;
      const reason = (
        await prompt({ title: one ? `Discard T${one.task}?` : `Discard ${on.length} tasks?`, placeholder: 'Why, in one line' })
      )?.trim();
      if (!reason) return null;
      const sure = await confirm({
        title: one ? `Discard “${one.title}”?` : `Discard these ${on.length} tasks?`,
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
        confirmText: one ? 'Discard it' : 'Discard them',
        cancelText: one ? 'Keep it' : 'Keep them',
        danger: true,
      });
      if (!sure) return null;
      return all(
        'discarded',
        on.map((x) => ({ keys: x.keys, run: () => verbs.discard(x.task, reason) })),
        true,
      );
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
  const name = bulkNames(b);
  // Done, the picks keep only the rows that failed; backing out of a cancel's questions keeps them all.
  const then = (run: Promise<string[] | null>) => void run.then((failed) => failed && pick?.keep(new Set(failed)));
  const keys = (on: { keys: string[] }[]) => on.flatMap((x) => x.keys);
  return {
    seen: b.seen.length ? { label: name.seen, keys: keys(b.seen), run: () => then(act.seen(b.seen)) } : null,
    later: b.later.length ? { label: name.later, keys: keys(b.later), run: (until: string) => then(act.later(b.later, until)) } : null,
    giveBack: b.giveBack.length ? { label: name.giveBack, keys: keys(b.giveBack), run: () => then(act.giveBack(b.giveBack)) } : null,
    done: b.close.length ? { label: name.done, keys: keys(b.close), run: () => then(act.done(b.close)) } : null,
    discard: b.close.length ? { label: name.discard, keys: keys(b.close), run: () => then(act.discard(b.close)) } : null,
    allow: b.answer.length ? { label: name.allow, keys: keys(b.answer), run: () => then(act.answer(b.answer, 'approve')) } : null,
    deny: b.answer.length ? { label: name.deny, keys: keys(b.answer), run: () => then(act.answer(b.answer, 'reject')) } : null,
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
      {b.done && (
        <MenuItem icon={<Check size={14} />} onClick={b.done.run}>
          {b.done.label}
        </MenuItem>
      )}
      {b.discard && (
        <MenuItem icon={<Ban size={14} />} danger onClick={b.discard.run}>
          {`${b.discard.label}…`}
        </MenuItem>
      )}
      <MenuSeparator />
      <MenuItem onClick={() => pick.clear()}>Clear the selection</MenuItem>
    </>
  );
}

export function BulkButtons() {
  const pick = usePick();
  const b = useBulk();
  const [pointed, setPointed] = useState<keyof typeof b | null>(null);
  const hinted = (pointed && b[pointed]?.keys.join('|')) || null;
  const setHint = pick?.setHint;
  useEffect(() => {
    setHint?.(hinted ? new Set(hinted.split('|')) : null);
    return () => setHint?.(null);
  }, [hinted, setHint]);
  const hint = (on: keyof typeof b) => ({
    onPointerEnter: () => setPointed(on),
    onPointerLeave: () => setPointed(null),
    onFocus: () => setPointed(on),
    onBlur: () => setPointed(null),
  });
  if (!b.seen && !b.later && !b.giveBack && !b.done && !b.discard && !b.allow)
    return <span className="px-2 whitespace-nowrap text-muted">Nothing to do to all of these</span>;
  return (
    <>
      {b.seen && (
        <Button variant="ghost" {...hint('seen')} onClick={b.seen.run}>
          <Eye size={14} />
          {b.seen.label}
        </Button>
      )}
      {b.later && (
        <Menu
          side="top"
          trigger={
            <Button variant="ghost" {...hint('later')}>
              <Clock size={14} />
              {b.later.label}
            </Button>
          }
        >
          <LaterItems onPick={b.later.run} />
        </Menu>
      )}
      {b.giveBack && (
        <Button variant="ghost" {...hint('giveBack')} onClick={b.giveBack.run}>
          <Undo2 size={14} />
          {b.giveBack.label}
        </Button>
      )}
      {b.done && (
        <Button variant="ghost" {...hint('done')} onClick={b.done.run}>
          <Check size={14} />
          {b.done.label}
        </Button>
      )}
      {b.discard && (
        <Button variant="ghost" {...hint('discard')} onClick={b.discard.run}>
          <Ban size={14} />
          {`${b.discard.label}…`}
        </Button>
      )}
      {b.deny && (
        <Button variant="ghost" {...hint('deny')} onClick={b.deny.run}>
          <X size={14} />
          {b.deny.label}
        </Button>
      )}
      {b.allow && (
        <Button variant="primary" {...hint('allow')} onClick={b.allow.run}>
          <Check size={14} />
          {b.allow.label}
        </Button>
      )}
    </>
  );
}

type RowOpens = { opens: string | null } & Omit<ComponentProps<typeof OpenItems>, 'opens'>;

/** A right-click on a row: its own actions, or, on a row among several picked, what can be done to all of them. */
export function RowMenu({
  row,
  open,
  children,
}: {
  row: Picked;
  /** What the row itself opens, as its click does. */
  open: RowOpens;
  children: ReactNode;
}) {
  const nameOf = useNameOf();
  const pick = usePick();
  const act = useRowActions();
  const mine = useMyTurns();
  const many = !!pick && pick.picked.size > 1 && pick.picked.has(row.key);
  const { p, r, key } = row;
  const task = p?.task ?? r?.task ?? null;
  const later = p ? laterKey(p) : null;
  const who = p?.agent ?? r?.now ?? r?.agent;
  const giveBack = !!p && task !== null && mine.has(task);
  const seen = !!r && task !== null && !!r.ended && !r.ended.seen;
  const close = !!r && task !== null && !r.ended;
  const acts = giveBack || !!later || seen || close;
  const items = many ? (
    <BulkItems />
  ) : (
    <>
      {open.opens && <OpenItems {...open} opens={open.opens} />}
      {who && open.opens !== chatOf(who) && (
        <SubMenu label={`${nameOf(who)}'s chat`} icon={<MessageSquare size={14} />}>
          <OpenItems opens={chatOf(who)} />
        </SubMenu>
      )}
      {(open.opens || who) && acts && <MenuSeparator />}
      {giveBack && task !== null && (
        <MenuItem icon={<Undo2 size={14} />} onClick={() => void act.giveBack([{ keys: [key], task }])}>
          Give back
        </MenuItem>
      )}
      {later && (
        <SubMenu label="Later" icon={<Clock size={14} />}>
          <LaterItems onPick={(until) => void act.later([{ keys: [key], later }], until)} />
        </SubMenu>
      )}
      {seen && task !== null && (
        <MenuItem icon={<Eye size={14} />} onClick={() => void act.seen([{ keys: [key], task }])}>
          Mark seen
        </MenuItem>
      )}
      {close && r && task !== null && (
        <>
          <MenuItem icon={<Check size={14} />} onClick={() => void act.done([{ keys: [key], task }])}>
            Done
          </MenuItem>
          <MenuItem icon={<Ban size={14} />} danger onClick={() => void act.discard([{ keys: [key], task, title: r.title }])}>
            Discard…
          </MenuItem>
        </>
      )}
    </>
  );
  return <ContextMenu items={items}>{children}</ContextMenu>;
}
