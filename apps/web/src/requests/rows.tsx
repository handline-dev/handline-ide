import { classify, sentence, useAgentDetail, usePromptAnswer, useProposals, useTask, useWrite } from '@handline/app-data';
import type { PendingRequest, Prompt, RequestRow } from '@handline/client';
import { tomorrowAt9 } from '@handline/app-core';
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
import { requestKey, RowMenu, turnKey, usePick, useRowActions, type Picked } from '#src/requests/RowMenu.tsx';
import { laterKey } from '#src/requests/data.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';
import { ago } from '#src/format/time.ts';
import { focusIn } from '#src/layout/focus.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
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
};

/** One row. In Requests (not the bell) it can be picked: Shift+click, or the check box that takes the icon's place on hover; then
 * a click picks more. A right-click opens its menu. A row dims while a write on it is on its way. */
function Line({
  title,
  meta,
  onOpen,
  mark,
  icon,
  row,
  opens,
  trailing,
}: {
  title: string;
  meta: string;
  onOpen: (how: 'here' | 'side') => void;
  /** What a drag of the row opens where it is dropped; none for a row that opens no tab. */
  opens?: () => string | null;
  /** Shown at the row's end while the pointer is on it: a dismiss, or a proposal's answer. */
  trailing?: ReactNode;
  mark?: boolean;
  icon: ReactNode;
  row: Picked;
}) {
  const pick = usePick();
  const picked = !!pick?.picked.has(row.key);
  const busy = !!pick?.busy.has(row.key);
  const open = openProps(onOpen, opens);
  const line = (
    // The check box sits beside the row's button, over its icon, not inside it: a box in a button is no box to the keyboard.
    <div
      className={`group/line relative flex rounded-md ${picked ? 'bg-selected' : 'hover:bg-hover/30'} ${busy ? 'pointer-events-none opacity-50' : ''}`}
    >
      {pick && (
        <input
          type="checkbox"
          aria-label={`Pick ${title}`}
          checked={picked}
          onChange={() => pick.toggle(row)}
          className={`absolute top-2.5 left-2 z-10 size-4 accent-accent ${picked ? '' : 'opacity-0 group-hover/line:opacity-100 focus-visible:opacity-100'}`}
        />
      )}
      <button
        type="button"
        {...open}
        onClick={(e: MouseEvent<HTMLButtonElement>) => {
          if (pick && (e.shiftKey || pick.picked.size > 0)) return pick.toggle(row);
          open.onClick?.(e);
        }}
        className="flex w-full gap-2.5 px-2 py-1.5 text-left"
      >
        <span
          className={`relative mt-0.5 grid size-4 shrink-0 place-items-center ${pick ? 'group-hover/line:invisible' : ''} ${picked ? 'invisible' : ''}`}
        >
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
  return pick ? <RowMenu row={row}>{line}</RowMenu> : line;
}

/** Where a row opens: its task; for a daemon warning, the account, the chat or the settings it is about; for a prompt, its agent's
 * chat; else Requests. */
function useOpenTurn(p: PendingRequest) {
  const openTab = useOpenTab();
  return (how: 'here' | 'side') => {
    const opens = turnOpens(p);
    if (opens) openTab(opens, how);
    else openSettings();
  };
}

/** The tab a row opens, or null for the disk warning, which opens Settings. */
function turnOpens(p: PendingRequest): string | null {
  const w = p.warning;
  if (p.task !== null) return taskOf(p.task);
  if (w?.account && w.kind === 'login') {
    focusIn('usage', '', w.account);
    return 'usage';
  }
  if (w?.agent) return chatOf(w.agent);
  if (w?.kind === 'disk') return null;
  if (p.source === 'prompt' && p.agent) return chatOf(p.agent);
  return 'requests';
}

/** Something waiting on the founder: what is asked, who asks (an agent, or the daemon itself), how long ago. */
export function TurnLine({ p, onOpened, until }: { p: PendingRequest; onOpened?: () => void; until?: string }) {
  const open = useOpenTurn(p);
  const daemon = p.kind === 'warning' || p.agent === null;
  return (
    <Line
      mark
      row={{ key: turnKey(p), p }}
      title={p.title}
      icon={turnIcon(p)}
      meta={[until ? `back ${until}` : ASK[p.kind], daemon ? 'daemon' : p.agent, p.task !== null && `T${p.task}`, p.ask, ago(p.last_at)]
        .filter(Boolean)
        .join(' · ')}
      opens={() => turnOpens(p)}
      trailing={<TurnQuick p={p} />}
      onOpen={(how) => {
        open(how);
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
  no_work: 'no work needed',
  not_yet: 'not started',
};

/** A finished request the founder has not opened since it ended. */
export const isUnseen = (r: RequestRow): boolean => r.ended?.seen === false;

/** One of the founder's requests: its title, who works on it now (or how it ended), when it last moved. A click opens its task, else
 * the chat it was said in; a result not seen yet is marked, and opening it marks it seen. */
export function RequestLine({ r, onOpened }: { r: RequestRow; onOpened?: () => void }) {
  const openTab = useOpenTab();
  const seen = useWrite('task.seen', r.task === null ? undefined : String(r.task));
  const who = r.now ?? r.holders[0] ?? r.agent;
  const closed = r.status === 'done' && r.task !== null;
  const task = useTask(r.task ?? 0, { enabled: closed });
  const failed = closed ? failedLine(task.data?.evidence, task.data?.title) : null;
  const ended = r.ended?.outcome === 'discarded' ? 'discarded' : STATUS[r.status];
  const meta = r.status === 'done' || r.status === 'no_work' ? [failed ?? ended, r.agent] : [who, r.progress || STATUS[r.status]];
  return (
    <Line
      mark={isUnseen(r)}
      row={{ key: requestKey(r), r }}
      icon={
        r.ended?.outcome === 'discarded' ? (
          <CircleSlash size={16} className="text-muted" />
        ) : failed ? (
          <CircleX size={16} className="text-danger" />
        ) : r.status === 'done' ? (
          <CircleCheck size={16} className="text-success" />
        ) : r.status === 'no_work' ? (
          <CircleCheck size={16} className="text-muted" />
        ) : (
          <CircleDashed size={16} className="text-muted" />
        )
      }
      title={r.title}
      meta={[...meta, ago(r.ended?.at ?? r.last_activity)].filter(Boolean).join(' · ')}
      opens={() => (r.task !== null ? taskOf(r.task) : chatOf(r.agent))}
      onOpen={(how) => {
        if (r.task !== null && isUnseen(r)) seen.mutate({ task: r.task });
        openTab(r.task !== null ? taskOf(r.task) : chatOf(r.agent), how);
        onOpened?.();
      }}
    />
  );
}

/** A permission prompt's Allow/Deny, as on its chat card: running the command another way leaves the agent waiting. */
function PromptQuick({ agent, id }: { agent: string; id: string }) {
  const prompt = useAgentDetail(agent).data?.prompt;
  if (!prompt || prompt.id !== id || prompt.kind !== 'permission') return null;
  return <PermissionAnswer agent={agent} p={prompt} />;
}

function PermissionAnswer({ agent, p }: { agent: string; p: Prompt }) {
  const a = usePromptAnswer(agent, p, () => toast('The prompt changed: open the chat to read it.', { tone: 'warning' }));
  const yes = a.opts.find((o) => o.answer === 'yes');
  const no = a.opts.find((o) => o.answer === 'no');
  if (!yes || !no) return null;
  return (
    <>
      <Button variant="primary" disabled={a.answering || a.changed} onClick={() => a.send(yes.key, yes.key)}>
        {a.mark(yes.key, 'Allow')}
      </Button>
      <Button disabled={a.answering || a.changed} onClick={() => a.send(no.key, no.key)}>
        {a.mark(no.key, 'Deny')}
      </Button>
    </>
  );
}

/** A proposal's or a permission prompt's answer, Allow or Deny, and a dismiss until tomorrow 09:00 (Later). A proposal's Allow names the
 *  subject proposal.list showed; one the daemon refuses as changed since stays off on this row, so it is opened and read before it is allowed. */
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
  return (
    <>
      {p.source === 'prompt' && p.agent && p.id && <PromptQuick agent={p.agent} id={p.id} />}
      {later && (
        <IconButton
          label="Dismiss until tomorrow 09:00"
          onClick={() => void act.later([{ key: turnKey(p), later }], tomorrowAt9(Date.now()))}
        >
          <X size={14} />
        </IconButton>
      )}
    </>
  );
}
