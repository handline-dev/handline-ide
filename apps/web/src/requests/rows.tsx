import { classify, sentence, useNameOf, useProposals, useTask, useWrite } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import { isUnseen, laterKey, requestRow, tomorrowAt9, turnKey, turnRow, type Picked } from '@handline/app-core';
import {
  CircleCheck,
  CircleX,
  CircleDashed,
  CircleSlash,
  HardDrive,
  KeyRound,
  Lightbulb,
  MailWarning,
  MessageCircleQuestion,
  MessageSquare,
  ShieldQuestion,
  Signpost,
  X,
} from 'lucide-react';
import { useState, type MouseEvent, type ReactNode } from 'react';
import { RowMenu, usePick, useRowActions } from '#src/requests/RowMenu.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ago } from '#src/format/time.ts';
import { focusIn } from '#src/layout/focus.ts';
import type { TaskRequest } from '#src/requests/data.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import type { How } from '#src/layout/openTab.tsx';
import { openSettings } from '#src/settings/open.ts';
import { goToTurnOf } from '#src/tools/TaskTurn.ts';
import { failedLine } from '#src/ui/taskLook.tsx';

const WARN: Record<NonNullable<PendingRequest['warning']>['kind'], typeof HardDrive> = {
  disk: HardDrive,
  login: KeyRound,
  delivery: MailWarning,
  undelivered: MailWarning,
};

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

const ASK: Record<PendingRequest['kind'], string> = {
  allow: 'Allow?',
  decide: 'Decide',
  ask: 'Question',
  warning: 'Warning',
  comment: 'New comments',
  note: 'Note',
};

const BOX_BESIDE = 'mt-2 ml-2';
const BOX_OVER = 'absolute top-2 left-2 z-10 opacity-0 group-hover/line:opacity-100 focus-visible:opacity-100';

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
}: {
  title: string;
  /** Where a click lands, shown while the pointer rests on the row. */
  hint?: string;
  meta: string;
  /** The tab the row opens; null for a row that opens no tab, only `here`. */
  opens: string | null;
  onOpen?: (how: How) => void;
  focus?: () => void;
  here?: () => void;
  /** Shown at the row's end while the pointer is on it: a dismiss, or a proposal's answer. */
  trailing?: ReactNode;
  mark?: boolean;
  icon: ReactNode;
  row: Picked;
}) {
  const pick = usePick();
  const picked = !!pick?.picked.has(row.key);
  let hinted = '';
  if (pick?.hint?.has(row.key)) hinted = 'ring-2 ring-accent ring-inset';
  else if (pick?.hint && picked) hinted = 'opacity-50';
  const busy = !!pick?.busy.has(row.key);
  const picking = !!pick?.picked.size;
  // In Requests the row has its own menu (RowMenu), with the open items at its top.
  const open = useOpenable(opens, { onOpen, focus, here, menu: !pick });
  const line = (
    // The check box sits beside the row's button, not inside it: a box in a button is no box to the keyboard.
    <div
      className={`group/line relative flex rounded-md ${picked ? 'bg-selected' : 'hover:bg-hover/30'} ${hinted} ${busy ? 'pointer-events-none opacity-50' : ''}`}
    >
      {pick && (
        <input
          type="checkbox"
          aria-label={`Pick ${title}`}
          checked={picked}
          onChange={() => pick.toggle(row)}
          className={`size-4 shrink-0 accent-accent ${picking ? BOX_BESIDE : BOX_OVER}`}
        />
      )}
      <button
        type="button"
        {...open}
        // While rows are picked, a click picks more and the other ways to open wait.
        onClick={(e: MouseEvent<HTMLButtonElement>) => {
          if (pick && (e.shiftKey || picking)) return pick.toggle(row);
          open.onClick(e);
        }}
        onAuxClick={picking ? undefined : open.onAuxClick}
        onKeyDown={picking ? undefined : open.onKeyDown}
        title={hint}
        className="flex w-full gap-2.5 px-2 py-1.5 text-left"
      >
        <span className={`relative mt-0.5 grid size-4 shrink-0 place-items-center ${pick && !picking ? 'group-hover/line:invisible' : ''}`}>
          {icon}
          {mark && <span className="absolute -top-0.5 -right-0.5 size-1.5 rounded-full bg-accent" />}
        </span>
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-fg">{title}</span>
          <span className="truncate text-xs text-muted">{meta}</span>
        </span>
      </button>
      {trailing && (
        <span className="absolute top-1.5 right-1.5 hidden items-center gap-1 rounded-md bg-surface group-hover/line:flex group-focus-within/line:flex">
          {trailing}
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
 * chat, or Requests. Null for the disk warning, which opens Settings. */
function turnOpens(p: PendingRequest): string | null {
  const w = p.warning;
  if (p.task !== null) return taskOf(p.task);
  if (w?.account && w.kind === 'login') return 'usage';
  if (w?.agent) return chatOf(w.agent);
  if (w?.kind === 'disk') return null;
  // A permission or a question without a task is asked in the agent's chat.
  return p.agent ? chatOf(p.agent) : 'requests';
}

/** Something waiting on the founder: what is asked, who asks (an agent, or the daemon itself), how long ago. */
export function TurnLine({ p, onOpened, until }: { p: PendingRequest; onOpened?: () => void; until?: string }) {
  const nameOf = useNameOf();
  const daemon = p.kind === 'warning' || p.agent === null;
  const opens = turnOpens(p);
  const login = p.warning?.kind === 'login' ? p.warning.account : undefined;
  return (
    <Line
      mark
      row={turnRow(p)}
      title={p.title}
      icon={turnIcon(p)}
      meta={[
        until ? `back ${until}` : ASK[p.kind],
        daemon ? 'daemon' : nameOf(p.agent!),
        p.task !== null && `T${p.task}`,
        p.ask,
        ago(p.last_at),
      ]
        .filter(Boolean)
        .join(' · ')}
      opens={opens}
      here={opens ? undefined : openSettings}
      trailing={<TurnQuick p={p} />}
      hint={
        p.task !== null && (p.kind === 'ask' || p.kind === 'decide')
          ? `Opens T${p.task} at ${p.agent ? `${nameOf(p.agent)}'s` : 'its'} question in Activity`
          : undefined
      }
      focus={login ? () => focusIn('usage', '', login) : undefined}
      onOpen={() => {
        // The task opens at what it asks of the founder, as the Your turn click does.
        if (p.task !== null && p.kind !== 'warning') goToTurnOf(p.task);
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

/** A proposal's answer, Allow or Deny; else a dismiss until tomorrow 09:00 (Later). An Allow names the subject proposal.list showed; one the
 *  daemon refuses as changed since stays off on this row, so a changed proposal is opened and read before it is allowed. */
function TurnQuick({ p }: { p: PendingRequest }) {
  const act = useRowActions();
  const decide = useWrite('proposal.decide');
  const listed = useProposals().data?.proposals.find((x) => String(x.id) === p.id);
  const [changed, setChanged] = useState(false);
  if (p.source === 'proposal' && p.id) {
    const answer = (decision: 'approve' | 'reject') =>
      decide.mutate(
        { proposal: Number(p.id), decision, ...(decision === 'approve' && listed ? { subject: listed.subject } : {}) },
        {
          onError: (e) => {
            const f = classify(e);
            if (f.kind === 'refused' && f.gate === 'proposal_changed') {
              setChanged(true);
              toast('The proposal changed since it was listed: open it and read it before you allow it.', { tone: 'warning' });
            } else if (sentence(f)) toast(`Did not answer: ${sentence(f)}`, { tone: 'warning' });
          },
        },
      );
    return (
      <>
        <Button
          variant="primary"
          disabled={decide.isPending || !listed || changed}
          title={changed ? 'Changed since it was listed: open it to read it' : listed ? undefined : 'Reading the proposal'}
          onClick={() => answer('approve')}
        >
          Allow
        </Button>
        <Button disabled={decide.isPending} onClick={() => answer('reject')}>
          Deny
        </Button>
      </>
    );
  }
  const later = laterKey(p);
  if (!later) return null;
  return (
    <IconButton
      label="Dismiss until tomorrow 09:00"
      onClick={() => void act.later([{ keys: [turnKey(p)], later }], tomorrowAt9(Date.now()))}
    >
      <X size={14} />
    </IconButton>
  );
}
