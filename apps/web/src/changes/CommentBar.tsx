// The bar over a diff while line comments wait as drafts: how many, and the one button that sends them all, whose words say what
// sending does there. A task's go on the task (a done one is opened again with them, an open one gets a comment, which is the answer
// while the person's own turn is on top); an agent's worktree has no task, so they go to its chat. A task's drafts are cleared once
// the comment is on the task; a chat's once the message is the chat's own, which keeps one that did not go and sends it again itself.
import { commentCount, lineCommentsSummary, lineCommentsText, taskCommentRoute } from '@handline/app-core';
import { useChatActions, useRead, useWrite, why } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { useState, type ReactNode } from 'react';
import { agentPlace, taskPlace, useLineDrafts } from '#src/changes/lineDrafts.ts';
import { Button } from '#src/ui/Button.tsx';
import { toast } from '#src/ui/toast.tsx';
import { Who } from '#src/ui/Who.tsx';

function Bar({ count, says, label, busy, onSend }: { count: number; says: ReactNode; label: string; busy: boolean; onSend: () => void }) {
  return (
    <div
      data-comment-bar
      className="sticky top-0 z-10 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-md border border-accent/40 bg-surface px-3 py-2 text-sm"
    >
      <span className="font-medium text-fg">{commentCount(count)}</span>
      <span className="min-w-0 flex-1 basis-40 text-xs text-muted">{says}</span>
      <Button variant="primary" busy={busy} onClick={onSend} className="ml-auto">
        {label}
      </Button>
    </div>
  );
}

const failed = (what: string) => (e: unknown) => toast(`${what}: ${why(e)}`, { tone: 'warning', ms: 8000 });

/** The drafts on a task's code. The whole text is kept as one comment on the task, which its agent is told in one cut line and reads
 *  whole with `hl task show`; a done task is then opened again, its reason the text's first line. */
export function TaskCommentBar({ task }: { task: number }) {
  const t = useRead('task.get', { task }).data;
  const me = useMe();
  const drafts = useLineDrafts(taskPlace(task));
  const add = useWrite('task.comment.add', String(task));
  const reopen = useWrite('task.reopen');
  const [busy, setBusy] = useState(false);
  const n = drafts.list.length;
  if (!t || !n) return null;
  const route = taskCommentRoute(t, me, n);
  const readWith = `hl task show T${task}`;
  const send = async () => {
    // What is sent is what is dropped once it went: a draft written while the send is on its way stays for the next one.
    const sent = drafts.now();
    setBusy(true);
    try {
      await add.mutateAsync({ task, text: lineCommentsText(sent, { readWith }) });
    } catch (e) {
      setBusy(false);
      return failed('The comments did not go')(e);
    }
    drafts.drop(sent.map((c) => c.id));
    if (route.via === 'reopen') {
      const note = lineCommentsSummary(sent, readWith);
      await reopen.mutateAsync({ task, note }).catch(failed('The comments are on the task, but it did not reopen'));
    }
    setBusy(false);
  };
  const says = route.answers ? (
    <>
      Sending gives <Who k={route.answers} /> your answer and the turn back
    </>
  ) : route.via === 'reopen' ? (
    'The task is done: sending opens it again, with these as one comment on it'
  ) : t.review?.ready ? (
    'Sent as one comment that asks its agent; the task stays Ready. Redo takes them too.'
  ) : (
    'Sent as one comment on the task'
  );
  return <Bar count={n} says={says} label={route.label} busy={busy} onSend={() => void send()} />;
}

/** The drafts on an agent's worktree changes: no task holds them, so they are one message in the agent's chat, sent as the chat's
 *  own box sends one. From then on the chat's row is the message: one that does not go stays there, said to have failed, with Retry
 *  (the same send again, never a second one) and Discard. So the drafts go as it is handed over, and nothing here can send it twice. */
export function ChatCommentBar({ agent }: { agent: string }) {
  const drafts = useLineDrafts(agentPlace(agent));
  const chat = useChatActions(agent);
  const n = drafts.list.length;
  if (!n) return null;
  return (
    <Bar
      count={n}
      says={
        <>
          Sent as one message to <Who k={agent} />; one that does not go waits in the chat to be tried again
        </>
      }
      label="Send to chat"
      busy={false}
      onSend={() => {
        const sent = drafts.now();
        chat.send(lineCommentsText(sent));
        drafts.drop(sent.map((c) => c.id));
      }}
    />
  );
}
