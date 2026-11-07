// Marked pictures made outside a chat (a Web tab's screenshot) on their way into an agent's next message: the agent's chat takes
// them when it is open, or as it opens.
import type { MarkedPicture } from '#src/viewer/ImageViewer.tsx';

const waiting = new Map<string, MarkedPicture[]>();
// Each agent's open chats, newest last: a chat open twice (two panes) gives way to the newer one, and back when it closes.
const takers = new Map<string, ((marked: MarkedPicture[]) => void)[]>();

export function toChat(agent: string, marked: MarkedPicture[]) {
  const take = takers.get(agent)?.at(-1);
  if (take) return take(marked);
  waiting.set(agent, [...(waiting.get(agent) ?? []), ...marked]);
}

/** The chat of `agent` takes what was sent to it, and what waited for it; returns the call that stops it. */
export function takeToChat(agent: string, take: (marked: MarkedPicture[]) => void): () => void {
  takers.set(agent, [...(takers.get(agent) ?? []), take]);
  const held = waiting.get(agent);
  waiting.delete(agent);
  if (held?.length) take(held);
  return () =>
    takers.set(
      agent,
      (takers.get(agent) ?? []).filter((t) => t !== take),
    );
}
