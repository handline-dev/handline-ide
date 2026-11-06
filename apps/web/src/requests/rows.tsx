import { useWrite } from '@handline/app-data';
import type { PendingRequest, RequestRow } from '@handline/client';
import {
  CircleCheck,
  CircleDashed,
  CircleSlash,
  HardDrive,
  Hourglass,
  KeyRound,
  Lightbulb,
  MailWarning,
  MessageCircleQuestion,
  MessageSquare,
  ShieldQuestion,
  Signpost,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { ago } from '#src/format/time.ts';
import { focusIn } from '#src/layout/focus.ts';
import { chatOf, taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { openSettings } from '#src/settings/open.ts';

const WARN: Record<NonNullable<PendingRequest['warning']>['kind'], typeof HardDrive> = {
  disk: HardDrive,
  login: KeyRound,
  weekly_use: Hourglass,
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

function Line({
  title,
  meta,
  onOpen,
  mark,
  icon,
}: {
  title: string;
  meta: string;
  onOpen: (how: 'here' | 'side') => void;
  mark?: boolean;
  icon: ReactNode;
}) {
  return (
    <button type="button" {...openProps(onOpen)} className="flex w-full gap-2.5 rounded-md px-2 py-1.5 text-left hover:bg-hover/30">
      <span className="relative mt-0.5 grid size-4 shrink-0 place-items-center">
        {icon}
        {mark && <span className="absolute -top-0.5 -right-0.5 size-1.5 rounded-full bg-accent" />}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-fg">{title}</span>
        <span className="truncate text-xs text-muted">{meta}</span>
      </span>
    </button>
  );
}

/** Where a row opens: its task; for a daemon warning, the account, the chat or the settings it is about; else Requests. */
function useOpenTurn(p: PendingRequest) {
  const openTab = useOpenTab();
  return (how: 'here' | 'side') => {
    const w = p.warning;
    if (p.task !== null) return openTab(taskOf(p.task), how);
    if (w?.account && (w.kind === 'login' || w.kind === 'weekly_use')) {
      focusIn('usage', '', w.account);
      return openTab('usage', how);
    }
    if (w?.agent) return openTab(chatOf(w.agent), how);
    if (w?.kind === 'disk') return openSettings();
    openTab('requests', how);
  };
}

/** Something waiting on the founder: what is asked, who asks (an agent, or the daemon itself), how long ago. */
export function TurnLine({ p, onOpened }: { p: PendingRequest; onOpened?: () => void }) {
  const open = useOpenTurn(p);
  const daemon = p.kind === 'warning' || p.agent === null;
  return (
    <Line
      mark
      title={p.title}
      icon={turnIcon(p)}
      meta={[ASK[p.kind], daemon ? 'daemon' : p.agent, p.task !== null && `T${p.task}`, p.ask, ago(p.last_at)].filter(Boolean).join(' · ')}
      onOpen={(how) => {
        open(how);
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
  const ended = r.ended?.outcome === 'discarded' ? 'discarded' : STATUS[r.status];
  const meta = r.status === 'done' || r.status === 'no_work' ? [ended, r.agent] : [who, r.progress || STATUS[r.status]];
  return (
    <Line
      mark={isUnseen(r)}
      icon={
        r.ended?.outcome === 'discarded' ? (
          <CircleSlash size={16} className="text-muted" />
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
      onOpen={(how) => {
        if (r.task !== null && isUnseen(r)) seen.mutate({ task: r.task });
        openTab(r.task !== null ? taskOf(r.task) : chatOf(r.agent), how);
        onOpened?.();
      }}
    />
  );
}
