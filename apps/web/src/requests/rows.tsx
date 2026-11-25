import { UnreadDot } from '#src/ui/Dot.tsx';
import { useNameOf, useProposals, useTask, useWrite } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import { isUnseen, pickGroup, proposalTitle, requestRow, turnRow, type Picked } from '@handline/app-core';
import type { HardDrive } from 'lucide-react';
import {
  CircleCheck,
  CircleX,
  CircleDashed,
  CircleSlash,
  ListChecks,
  Lightbulb,
  MailWarning,
  MessageCircleQuestion,
  MessageSquare,
  ShieldQuestion,
  Signpost,
  SquareArrowOutUpRight,
  UserRound,
} from 'lucide-react';
import { motion } from 'framer-motion';
import { useEffect, type MouseEvent, type ReactNode } from 'react';
import { RowMenu, usePick } from '#src/requests/RowMenu.tsx';
import { TurnQuick } from '#src/requests/TurnQuick.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ago } from '#src/format/time.ts';
import { focusIn } from '#src/layout/focus.ts';
import type { TaskRequest } from '#src/requests/data.ts';
import { chatOf, proposalOf, taskOf } from '#src/layout/kinds.tsx';
import { useNewTab, useOpenable } from '#src/layout/openable.tsx';
import type { Gesture } from '#src/layout/openTab.tsx';
import { openSettings, settingsFor } from '#src/settings/open.ts';
import { useCluster } from '#src/connection/clusters.tsx';
import { goToTurnOf } from '#src/tools/TaskTurn.ts';
import { failedLine } from '#src/ui/taskLook.tsx';
import { WARN, WARN_WORD } from '#src/requests/warnings.ts';
import { useRefs } from '#src/ui/refs.tsx';
import { spring } from '#src/motion.ts';

const KIND: Record<PendingRequest['kind'], typeof HardDrive> = {
  allow: ShieldQuestion,
  decide: Signpost,
  ask: MessageCircleQuestion,
  warning: MailWarning,
  comment: MessageSquare,
  note: MessageSquare,
};

/** What kind of thing waits on the founder, at a glance; the word in the grey line says it too. */
function turnIcon(p: PendingRequest): ReactNode {
  if (p.warning) {
    const W = WARN[p.warning.kind];
    return <W size={16} className="text-warning" />;
  }
  const K = p.source === 'proposal' ? Lightbulb : KIND[p.kind];
  return <K size={16} className="text-muted" />;
}

/** In Requests, what a row is about: a task, an account, or the warning's own thing. */
function subjectIcon(p: PendingRequest): ReactNode {
  if (p.task !== null) return <ListChecks size={16} className="text-muted" />;
  if (p.source === 'proposal') return <Lightbulb size={16} className="text-muted" />;
  if (p.warning?.kind === 'login') return <UserRound size={16} className="text-warning" />;
  return turnIcon(p);
}

const ASK: Record<PendingRequest['kind'], string> = {
  allow: 'Allow?',
  decide: 'Decide',
  ask: 'Question',
  warning: 'Warning',
  comment: 'New comments',
  note: 'Note',
};

// The box sits at the row's left: over the icon on hover, and while rows are picked in a gap that opens beside it.
const BOX = 'absolute top-2 left-2 z-10';
const BOX_HIDDEN = 'opacity-0 group-hover/line:opacity-100 focus-visible:opacity-100';

/** One row. In Requests (not the bell) it can be picked: Shift+click, or its check box, over the icon on hover and left of it while
 * any row is picked, when a click picks more. A right-click opens its menu. A row dims while a write on it is on its way. */
function Line({
  title,
  meta,
  opens,
  onOpen,
  focus,
  here,
  mark,
  icon,
  row,
  trailing,
  hint,
  lead,
  who,
  wait,
  answers,
  onPick,
  chosen,
  word,
}: {
  /** Another cluster's name, at the row's end. */
  word?: ReactNode;
  /** A plain click calls this instead of opening; `chosen` marks the shown row. */
  onPick?: () => void;
  chosen?: boolean;
  title: string;
  /** Where a click lands, shown while the pointer rests on the row. */
  hint?: string;
  meta: string;
  /** The tab the row opens; null for a row that opens no tab, only `here`. */
  opens: string | null;
  onOpen?: (how: Gesture) => void;
  focus?: () => void;
  here?: () => void;
  /** Shown at the row's end while the pointer is on it: a dismiss, or a proposal's answer. */
  trailing?: ReactNode;
  mark?: boolean;
  icon: ReactNode;
  row: Picked;
  /** In Requests' own list: the task number (or HR) in its own column, who asked after the title, the wait at the end, and answers
   * always shown under it. */
  lead?: string;
  who?: string;
  wait?: string;
  answers?: ReactNode;
}) {
  const pick = usePick();
  const picked = !!pick?.picked.has(row.key);
  let hinted = '';
  if (pick?.hint?.has(row.key)) hinted = 'ring-2 ring-accent ring-inset';
  else if (pick?.hint && picked) hinted = 'opacity-50';
  const busy = !!pick?.busy.has(row.key);
  const picking = !!pick?.picked.size;
  // While one kind is picked, the other kind dims: it cannot join this pick.
  const first = pick?.picked.values().next().value;
  const apart = !!first && pickGroup(first) !== pickGroup(row);
  // In Requests the row has its own menu (RowMenu), with the open items at its top.
  const open = useOpenable(opens, { onOpen, focus, here, menu: !pick });
  const listed = pick?.listed;
  useEffect(() => {
    if (!listed) return;
    listed.set(row.key, row);
    return () => void listed.delete(row.key);
  }, [listed, row]);
  const newTab = useNewTab(opens, { onOpen, focus, here });
  const line = (
    // The check box sits beside the row's button, not inside it: a box in a button is no box to the keyboard.
    <div
      data-pick-key={pick ? row.key : undefined}
      className={`group/line relative flex ${lead !== undefined ? 'border-b border-border' : 'rounded-md'} ${picked || chosen ? 'bg-selected' : 'hover:bg-hover/30'} ${hinted} ${busy ? 'pointer-events-none opacity-50' : apart ? 'opacity-40' : ''}`}
    >
      {pick && (
        <input
          type="checkbox"
          aria-label={`Pick ${title}`}
          checked={picked}
          onChange={() => undefined}
          // A Shift-click picks every row from the last one toggled to this one.
          onClick={(e) => (apart ? toast(APART, { tone: 'warning' }) : e.shiftKey ? pick.range(row) : pick.toggle(row))}
          className={`size-4 shrink-0 accent-accent ${BOX} ${picking ? '' : BOX_HIDDEN}`}
        />
      )}
      {pick && <motion.span aria-hidden initial={false} animate={{ width: picking ? 24 : 0 }} transition={spring} className="shrink-0" />}
      <button
        type="button"
        {...open}
        // A click opens the row, picked or not: only its box picks.
        onClick={(e: MouseEvent<HTMLButtonElement>) => {
          if (pick && apart) return void toast(APART, { tone: 'warning' });
          if (onPick && !e.metaKey && !e.ctrlKey && !e.altKey) return onPick();
          open.onClick(e);
        }}
        onAuxClick={open.onAuxClick}
        onKeyDown={open.onKeyDown}
        title={hint}
        className={`flex min-w-0 flex-1 gap-2.5 text-left ${lead !== undefined ? 'px-2 py-3' : 'px-2 py-1.5'}`}
      >
        {lead !== undefined ? (
          // A Requests row: the task number in the title's line, the text under both from the row's left edge.
          <span className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="flex min-w-0 items-baseline gap-2">
              <span
                className={`grid h-lh w-4 shrink-0 self-start place-items-center ${pick && !picking ? 'group-hover/line:invisible' : ''}`}
              >
                {icon}
              </span>
              {lead && <span className="shrink-0 text-muted tabular-nums">{lead}</span>}
              <span className="truncate font-medium text-fg">{title}</span>
              {who && <span className="shrink-0 text-muted">{who}</span>}
              {wait && <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">{wait}</span>}
              {word && <span className={`flex shrink-0 items-center ${wait ? '' : 'ml-auto pl-3'}`}>{word}</span>}
            </span>
            {/* The answers sit over the row's bottom-right: the text keeps their room. */}
            {meta && <span className={`line-clamp-2 break-words text-muted ${answers ? 'pr-36' : ''}`}>{meta}</span>}
          </span>
        ) : (
          <>
            <span
              className={`relative mt-0.5 grid size-4 shrink-0 place-items-center ${pick && !picking ? 'group-hover/line:invisible' : ''}`}
            >
              {icon}
              {mark && <UnreadDot className="absolute -top-0.5 -right-0.5" />}
            </span>
            <span className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="flex min-w-0 items-baseline gap-2">
                <span className="truncate text-fg">{title}</span>
                {who && <span className="shrink-0 text-muted">{who}</span>}
                {wait && <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">{wait}</span>}
              </span>
              <span className="line-clamp-2 break-words text-xs text-muted">{meta}</span>
            </span>
          </>
        )}
      </button>
      {answers && <span className="absolute right-2 bottom-2 flex items-center gap-2">{answers}</span>}
      {(trailing || (opens && !picking)) && (
        <span className="absolute top-1.5 right-1.5 hidden items-center gap-1 rounded-md bg-surface group-hover/line:flex group-focus-within/line:flex">
          {trailing}
          {opens && !picking && (
            <IconButton className="size-6!" label="Open in new tab" onClick={newTab}>
              <SquareArrowOutUpRight size={14} />
            </IconButton>
          )}
        </span>
      )}
    </div>
  );
  return pick ? (
    <RowMenu row={row} open={{ opens, onOpen, focus, here }}>
      {line}
    </RowMenu>
  ) : (
    line
  );
}

/** Where a row opens: its task; for a daemon warning, the account, the chat or the settings it is about; else the asking agent's
 * chat, or Requests. Null for the disk and review warnings, which open Settings. */
function turnOpens(p: PendingRequest, asker?: string | null): string | null {
  const w = p.warning;
  if (p.task !== null) return taskOf(p.task);
  // A proposal is answered in the chat of the agent that asked, where its card waits; one the daemon made opens on its own.
  if (p.source === 'proposal' && p.id) return asker ? chatOf(asker) : proposalOf(Number(p.id));
  if (w?.account && w.kind === 'login') return 'usage';
  if (w?.kind === 'disk' || w?.kind === 'review') return null;
  if (w?.agent) return chatOf(w.agent);
  // A permission or a question without a task is asked in the agent's chat.
  return p.agent ? chatOf(p.agent) : 'requests';
}

/** In Requests' own list, a proposal's and a step-down offer's answers always show; the others' on hover. */
const answersShown = (p: PendingRequest) => p.source === 'proposal' || p.warning?.kind === 'review';

/** Something waiting on the founder: what is asked, who asks (an agent, or the daemon itself), how long ago. */
export function TurnLine({
  p,
  onOpened,
  until,
  full,
  onPick,
  chosen,
  word,
}: {
  word?: ReactNode;
  p: PendingRequest;
  onOpened?: () => void;
  until?: string;
  full?: boolean;
  onPick?: () => void;
  chosen?: boolean;
}) {
  const nameOf = useNameOf();
  const refs = useRefs();
  const cluster = useCluster();
  const daemon = p.kind === 'warning' || p.agent === null;
  // A proposal is called by what it does, under it its reason; the daemon's title is `<kind> <agent>`.
  const proposal = useProposals().data?.proposals.find((x) => p.source === 'proposal' && String(x.id) === p.id);
  const opens = turnOpens(p, proposal?.requested_by !== 'human' ? proposal?.requested_by : null);
  const login = p.warning?.kind === 'login' ? p.warning.account : undefined;
  return (
    <Line
      mark
      row={turnRow(p)}
      word={word}
      onPick={onPick}
      chosen={chosen}
      title={proposal ? proposalTitle(proposal, nameOf) : refs(p.title)}
      icon={full ? subjectIcon(p) : turnIcon(p)}
      {...(full && {
        lead: p.source === 'proposal' ? '' : p.task !== null ? `T${p.task}` : p.warning ? WARN_WORD[p.warning.kind] : '',
        who: proposal ? (proposal.requested_by ? nameOf(proposal.requested_by) : 'daemon') : daemon ? 'daemon' : nameOf(p.agent!),
        wait: until ? `back ${until}` : ago(p.last_at),
        answers: answersShown(p) ? <TurnQuick p={p} /> : undefined,
      })}
      meta={
        full
          ? proposal?.reason
            ? proposal.reason
            : p.ask
              ? refs(p.ask)
              : p.warning
                ? (p.warning.line ?? '') // the icon and its word already say what it is; a step-down offer adds its record
                : ASK[p.kind]
          : [
              until ? `back ${until}` : ASK[p.kind],
              daemon ? 'daemon' : nameOf(p.agent!),
              p.task !== null && `T${p.task}`,
              p.ask && refs(p.ask),
              ago(p.last_at),
            ]
              .filter(Boolean)
              .join(' · ')
      }
      opens={opens}
      here={opens ? undefined : () => openSettings({ ...settingsFor(p.warning), cluster })}
      trailing={full && answersShown(p) ? undefined : <TurnQuick p={p} />}
      hint={
        p.task !== null && (p.kind === 'ask' || p.kind === 'decide')
          ? `Opens T${p.task} at ${p.agent ? `${nameOf(p.agent)}'s` : 'its'} question in Activity`
          : undefined
      }
      focus={login ? () => focusIn('usage', '', login) : undefined}
      onOpen={() => {
        // The task opens at what it asks of the founder, as the Your turn click does; a note opens it where it was read last.
        if (p.task !== null && p.kind !== 'warning' && p.kind !== 'note') goToTurnOf(p.task);
        onOpened?.();
      }}
    />
  );
}

const STATUS: Record<RequestRow['status'], string> = {
  waiting_on_you: 'waiting on you',
  review: 'in review',
  doing: 'in progress',
  done: 'done',
};

/** One of the founder's requests: its title, who works on it now (or how it ended), when it last moved. A click opens its task; a
 * result not seen yet is marked, and opening it marks it seen. */
export function RequestLine({ r, onOpened }: { r: TaskRequest; onOpened?: () => void }) {
  const seen = useWrite('task.seen', String(r.task));
  const nameOf = useNameOf();
  const who = nameOf(r.now ?? r.holders[0] ?? r.agent);
  const closed = r.status === 'done';
  const task = useTask(r.task, { enabled: closed });
  const failed = closed ? failedLine(task.data?.evidence, task.data?.title) : null;
  const ended = r.ended?.outcome === 'discarded' ? 'discarded' : STATUS[r.status];
  const meta = r.status === 'done' ? [failed ?? ended, nameOf(r.agent)] : [who, r.progress || STATUS[r.status]];
  return (
    <Line
      mark={isUnseen(r)}
      row={requestRow(r)}
      icon={
        r.ended?.outcome === 'discarded' ? (
          <CircleSlash size={16} className="text-muted" />
        ) : failed ? (
          <CircleX size={16} className="text-danger" />
        ) : r.status === 'done' ? (
          <CircleCheck size={16} className="text-success" />
        ) : (
          <CircleDashed size={16} className="text-muted" />
        )
      }
      title={r.title}
      meta={[...meta, ago(r.ended?.at ?? r.last_activity)].filter(Boolean).join(' · ')}
      opens={taskOf(r.task)}
      onOpen={() => {
        if (isUnseen(r)) seen.mutate({ task: r.task });
        onOpened?.();
      }}
    />
  );
}

/** Said when a dimmed row is clicked while the other kind is picked. */
const APART = 'Pick one kind at a time.';
