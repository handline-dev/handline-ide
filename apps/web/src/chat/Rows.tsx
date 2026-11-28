// A chat's rows, from its messages and transcript to what is drawn: the live stream and a found message's window draw the same lines.
import { chatView, foldBash, foldTurns, onePerTask, taskRuns, type ChatSource, type TaskEvent } from '@handline/app-core';
import { useAgentDetail, useNameOf } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { AnimatePresence, motion } from 'framer-motion';
import { moveNoteAfter } from '#src/chat/agent/AccountSwitch.tsx';
import { ChatLine, type Reply } from '#src/chat/ChatLine.tsx';
import { TaskActivity } from '#src/chat/TaskActivity.tsx';
import { StepRows } from '#src/chat/StepRows.tsx';
import { TurnRecord } from '#src/chat/TurnRecord.tsx';
import { spring } from '#src/motion.ts';

const NO_EVENTS: TaskEvent[] = [];

/** The lines of `agent`'s chat as it draws them: each task once, as a row where it first came up. `live`: the task changes heard since
 * the chat opened, which the live stream adds at its end. */
export function useChatRows(source: ChatSource, agent: string, live: TaskEvent[] = NO_EVENTS) {
  const nameOf = useNameOf();
  const me = useMe();
  // Drawn by the chat's own rules (mine) whatever was read: what the agent says to other agents shows as quiet lines.
  const { entries, bash } = foldBash(chatView(source, 'mine', agent, me, [], true).entries);
  return {
    entries,
    /** What each ! line ran and printed, by its message. */
    bash,
    rows: onePerTask(taskRuns(foldTurns(entries), agent, me, nameOf, live)),
    /** The words of each loaded message, for the line above a reply. */
    words: new Map(entries.flatMap((e) => (e.kind === 'message' ? [[e.message.id, e.message.body] as const] : []))),
    me,
  };
}

/** The rows drawn. Task notices in a row are one card, as the live task card is (TaskActivity). Only rows after `known` fade in: an
 * older page lands as one still block. Without `onReply` a line offers no reply (a window that has no box to write in). */
export function ChatRows({
  lines,
  agent,
  known = Infinity,
  onReply,
}: {
  lines: ReturnType<typeof useChatRows>;
  agent: string;
  known?: number;
  onReply?: (r: Reply) => void;
}) {
  const { rows, bash, words, me } = lines;
  // A Chat agent has no terminal: its chat shows every step, not one folded line.
  const steps = useAgentDetail(agent).data?.settings?.session_mode_in_force === 'chat';
  return (
    <AnimatePresence initial={false}>
      {rows.map((r, i, all) =>
        'events' in r ? (
          <motion.div
            key={r.id}
            data-id={r.key}
            initial={i <= known ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={spring}
            className="flex"
          >
            <TaskActivity events={r.events} />
          </motion.div>
        ) : r.kind === 'record' ? (
          steps ? (
            <StepRows key={`record:${r.id}`} items={r.items.filter((x) => !(x.kind === 'prompt' && x.open))} />
          ) : (
            <TurnRecord key={`record:${r.id}`} r={r} fade={i > known} />
          )
        ) : (
          <ChatLine
            key={`${r.kind}:${r.id}`}
            e={r}
            agent={agent}
            me={me}
            onReply={onReply}
            parentText={r.kind === 'message' && r.message.parent != null ? words.get(r.message.parent) : undefined}
            bash={r.kind === 'message' ? bash.get(r.message.id) : undefined}
            moveNote={r.kind === 'item' && r.item.account_moved ? moveNoteAfter(all, i) : undefined}
            fade={i > known}
          />
        ),
      )}
    </AnimatePresence>
  );
}
