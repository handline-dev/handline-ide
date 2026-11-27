// Marked pictures made outside a chat (a Web tab's screenshot, or an element picked in its page, whose details ride on the picture
// as its `note`) on their way into an agent's next message: the agent's chat takes them when it is open, or as it opens.
import type { MarkedPicture } from '#src/viewer/ImageViewer.tsx';

type Take = (marked: MarkedPicture[], send: boolean) => void;
const waiting = new Map<string, { marked: MarkedPicture[]; send: boolean }>();
// Each agent's open chats, newest last: a chat open twice (two panes) gives way to the newer one, and back when it closes.
const takers = new Map<string, Take[]>();

/** The kept name of the agent whose chat was shown last: where a Web tab sends when no agent is on screen. */
export const LAST_CHAT = 'chat/last';

export function toChat(agent: string, marked: MarkedPicture[], send = false) {
  const take = takers.get(agent)?.at(-1);
  if (take) return take(marked, send);
  const held = waiting.get(agent);
  waiting.set(agent, { marked: [...(held?.marked ?? []), ...marked], send: send || !!held?.send });
}

/** The chat of `agent` takes what was sent to it, and what waited for it; returns the call that stops it. */
export function takeToChat(agent: string, take: Take): () => void {
  takers.set(agent, [...(takers.get(agent) ?? []), take]);
  const held = waiting.get(agent);
  waiting.delete(agent);
  if (held?.marked.length) take(held.marked, held.send);
  return () =>
    takers.set(
      agent,
      (takers.get(agent) ?? []).filter((t) => t !== take),
    );
}
