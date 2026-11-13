// The box under a task: a comment (the chat's own box, with @ and / and files), who it will reach, and someone to hand the task to with it.
import { foundersTurn, refsIn, renderRefs, toRefs, withImages } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { classify, hooks, sentence, useAgents, useDirectory, useGiveBack, useNameOf, useServes, useWrite } from '@handline/app-data';
import { Who } from '#src/ui/Who.tsx';
import { Check, CornerDownRight, Eye, Flag, Timer, Trash2, UserRound, X } from 'lucide-react';
import { useState } from 'react';
import type { Attachments } from '#src/chat/attachments.tsx';
import { Composer } from '#src/chat/Composer.tsx';
import type { MarkedPicture } from '#src/viewer/ImageViewer.tsx';
import type { Command } from '#src/chat/Suggest.tsx';
import { AssignPicker } from '#src/tools/TaskFields.tsx';
import { openAsks } from '#src/tools/TaskWaiting.tsx';
import { turnReply } from '#src/tools/TaskTurn.ts';
import { confirm } from '#src/ui/modal.tsx';
import { useMe } from '@handline/app-react';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Page } from '#src/ui/Page.tsx';

import { sayRefusal } from '#src/ui/refusal.tsx';
import { toast } from '#src/ui/toast.tsx';

/** What the box answers: a comment, or the founder's turn (tied to the comment that asked). */
export type Reply = { id?: number; author: string; text: string; turn?: boolean };

/** Who a comment reaches by its own words: the assignee and each agent it names, `text` as it is sent (toRefs). */
const readers = (owner: string, text: string, known: string[]) => [...new Set([owner, ...refsIn(text).filter((n) => known.includes(n))])];

/** How many a comment will reach; hovering names them. */
function Readers({ names }: { names: string[] }) {
  return (
    <HoverCard
      align="end"
      trigger={
        <button type="button" className="flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted hover:bg-hover/30 hover:text-fg">
          <Eye size={14} />
          {names.length}
        </button>
      }
    >
      <div className="flex flex-col gap-1 p-2 text-sm">
        <span className="text-xs text-muted">This comment reaches</span>
        {names.map((n) => (
          <span key={n}>
            <Who k={n} />
          </span>
        ))}
      </div>
    </HoverCard>
  );
}

export function CommentBox({
  t,
  reply: asked,
  typing,
  onDrop,
  attach,
  draft,
  setDraft,
  onMarked,
  onSent,
}: {
  t: TaskDetail;
  reply: Reply | null;
  /** Changed when the box is to take the keys (Answer in task). */
  typing?: number;
  onDrop: () => void;
  attach: Attachments;
  draft: string;
  setDraft: (next: string | ((d: string) => string)) => void;
  onMarked: (marked: MarkedPicture[]) => void;
  onSent: () => void;
}) {
  const me = useMe();
  const dir = useDirectory();
  const nameOf = useNameOf();
  // On the founder's turn the box answers it, unless a comment was picked to reply to.
  const turn = foundersTurn(t, me) ? t.turns?.at(-1) : undefined;
  const picked = asked?.turn ? null : asked;
  const reply = picked ?? (turn ? turnReply(t) : null);
  const back = useGiveBack();
  const giveBack = async () => {
    if (
      !(await confirm({
        title: 'Give it back with no answer?',
        message: `${nameOf(turn?.by ?? '')} goes on without one.`,
        confirmText: 'Give back',
      }))
    )
      return;
    back.give(t.id, { onError: (e) => toast(`Give back did not work: ${sentence(classify(e))}`, { tone: 'warning' }) });
  };
  const read = useWrite('inbox.mark_read');
  const box = `task:${t.id}`;
  const add = useWrite('task.comment.add', String(t.id));
  const edit = useWrite('task.edit');
  // Someone to hand it to with this comment; none keeps the owner.
  const [to, setTo] = useState<string | null>(null);
  const known = useAgents().data?.agents.map((a) => a.key) ?? [];
  const sent = toRefs(dir, draft);
  // Who it reaches, as the daemon will tell it (the assignee, each @name, and for the founder on a task no agent holds who it answers to); asked again
  // only when the @names change. A handover with it, or a daemon without the call, falls back to the assignee and the @names.
  const named = readers('', sent, known)
    .filter(Boolean)
    .map((k) => `<@${k}>`)
    .join(' ');
  const served = useServes('task.comment.recipients').served;
  const told = hooks.useTaskCommentRecipients({ task: t.id, text: named }, { enabled: served && !to }).data?.recipients;
  const reach = !to && told ? told.map((r) => r.agent) : readers(to ?? t.to, sent, known);
  const done = useWrite('task.done');
  const discard = useWrite('task.discard');
  const change = (what: string, p: { owner?: string; priority?: 1 | 2 | 3; time_box?: string }) =>
    edit.mutate({ task: t.id, ...p }, { onError: failed(`${what} did not change`) });
  const commands: Command[] = [
    {
      name: 'done',
      hint: 'Mark it done',
      icon: <Check size={14} />,
      run: () => done.mutate({ task: t.id }, { onError: sayRefusal }),
    },
    {
      name: 'assign',
      hint: 'Give it to someone',
      takes: 'name',
      icon: <UserRound size={14} />,
      run: (a) => change('Assignee', { owner: a.replace(/^@/, '') }),
    },
    {
      name: 'priority',
      hint: '1, 2 or 3',
      takes: '1-3',
      icon: <Flag size={14} />,
      run: (a) =>
        /^[123]$/.test(a) ? change('Priority', { priority: Number(a) as 1 | 2 | 3 }) : toast('Priority is 1, 2 or 3.', { tone: 'warning' }),
    },
    {
      name: 'timebox',
      hint: 'Like 45m, 2h, 1d',
      takes: 'length',
      icon: <Timer size={14} />,
      run: (a) =>
        /^(\d+d)?(\d+h)?(\d+m)?$/.test(a) && a
          ? change('Time box', { time_box: a })
          : toast('A length like 45m, 2h or 1d.', { tone: 'warning' }),
    },
    {
      name: 'discard',
      hint: 'Discard it',
      icon: <Trash2 size={14} />,
      run: () => discard.mutate({ task: t.id }, { onError: sayRefusal }),
    },
  ];
  const failed = (what: string) => (e: unknown) => toast(`${what}: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
  const send = (body: string, ids: string[] = []) => {
    // The comment first, then the handover: the new owner finds it waiting.
    // An answer to a comment that asked the founder ends the wait: the open asks are closed with it.
    const answers = reply?.text.includes(`<@${me}>`) ? openAsks(t, me).map((r) => r.id) : [];
    onSent();
    add.mutate(
      { task: t.id, text: withImages(toRefs(dir, body), ids), ...(reply?.id != null ? { reply: reply.id } : {}) },
      {
        onError: failed('Comment did not go'),
        onSuccess: () => {
          if (to) edit.mutate({ task: t.id, owner: to }, { onError: failed('Assignee did not change') });
          if (answers.length) read.mutate({ ids: answers });
        },
      },
    );
    setTo(null);
    onDrop();
  };
  // Answering the founder's turn, not handing the task on: the box ends the turn, and empty it gives the turn back with no words.
  const answering = !!turn && !picked && !to;
  return (
    <Page>
      <Composer
        agent={box}
        commands={commands}
        sendLabel={answering ? 'Answer' : undefined}
        onEmpty={answering ? { label: 'Give back', run: () => void giveBack() } : undefined}
        locked={back.pending ? 'Giving it back' : undefined}
        focusOn={picked ?? (typing || undefined)}
        above={
          answering ? (
            <span className="flex items-center gap-1.5 text-xs text-muted">
              <CornerDownRight size={14} className="shrink-0" />
              Sending gives <Who k={turn.by} /> your answer and the turn back · Send it empty to give it back with no answer
            </span>
          ) : (
            picked && (
              <div role="status" className="-ml-2 flex min-w-0 items-center gap-1.5 text-xs text-muted">
                <CornerDownRight size={14} className="ml-1 shrink-0" />
                <span className="min-w-0 flex-1 truncate">
                  {`Replying to ${nameOf(picked.author)}: ${renderRefs(dir, picked.text.replaceAll(`<@${me}>`, ''))
                    .replace(/\s+/g, ' ')
                    .trim()}`}
                </span>
                <IconButton className="size-6!" label="Drop the reply" onClick={onDrop}>
                  <X size={14} />
                </IconButton>
              </div>
            )
          )
        }
        placeholder={
          to
            ? `Comment and assign to ${nameOf(to)}`
            : answering
              ? `Your answer to ${nameOf(turn.by)}`
              : t.to
                ? `Reply to ${nameOf(t.to)} on T${t.id}`
                : `Comment on T${t.id}`
        }
        draft={draft}
        setDraft={setDraft}
        attach={attach}
        onMarked={onMarked}
        leading={
          <AssignPicker
            t={t}
            onPick={setTo}
            trigger={
              <button
                type="button"
                className="flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted hover:bg-hover/30 hover:text-fg"
              >
                <UserRound size={14} />
                {to ? `Assign to ${nameOf(to)}` : 'Assign'}
              </button>
            }
          />
        }
        trailing={
          <>
            <Readers names={reach} />
            {to && (
              <IconButton label="Keep the assignee" onClick={() => setTo(null)}>
                <X size={14} />
              </IconButton>
            )}
          </>
        }
        onSend={send}
      />
    </Page>
  );
}
