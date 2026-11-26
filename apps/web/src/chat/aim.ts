// A message a chat is asked to show from outside it: the list of schedules opens the chat on the message a schedule was last sent as.
// The agent's chat takes the aim once it is drawn: it reads older pages until the message is among its lines, puts it in the middle of
// the view and lights it up once.
import { useEffect, useRef, type RefObject } from 'react';
import { createStore } from '#src/persist/store.ts';
import { toast } from '#src/ui/toast.tsx';

type Aim = { agent: string; message: number; at: string; asked: number };
const aim = createStore<Aim | null>(null);

/** An aim no chat took in this long is dropped: its chat did not open. */
const KEEP_MS = 20_000;
/** The most older pages one aim reads; a message further back is not shown. */
export const AIM_PAGES = 40;

/** Asks `agent`'s chat to show the message numbered `message`, sent at `at`. */
export const aimChat = (agent: string, message: number, at: string) => aim.set({ agent, message, at, asked: Date.now() });

/**
 * What a chat does about a message sent at `at` that is not among its lines. `floor` is the time its lines are drawn from (null: from
 * the first one): a message at or after it would be drawn, so it is `gone` from this chat; an older one needs an `older` page, after
 * the one being read (`wait`), until there is none or `pages` of them were read (`far`).
 */
export function aimStep(c: {
  floor: string | null;
  at: string;
  hasOlder: boolean;
  loadingOlder: boolean;
  pages: number;
}): 'gone' | 'wait' | 'older' | 'far' {
  if (c.floor === null || Date.parse(c.floor) <= Date.parse(c.at)) return 'gone';
  if (c.loadingOlder) return 'wait';
  return c.hasOlder && c.pages < AIM_PAGES ? 'older' : 'far';
}

/** Puts the message's line mid-view and lights it up; false when it is not drawn. The DOM writes live here, outside the hook. */
function show(box: HTMLElement, message: number): boolean {
  const line = box.querySelector(`[data-id="message:${message}"]`);
  if (!line) return false;
  line.scrollIntoView({ block: 'center' });
  line.classList.add('just-changed');
  setTimeout(() => line.classList.remove('just-changed'), 2500);
  return true;
}

/** In an agent's chat: takes an aim at this agent, reading older pages (`chat.loadOlder`) until its message is drawn. */
export function useChatAim(
  agent: string,
  stream: RefObject<HTMLDivElement | null>,
  chat: { floor: string | null; hasOlder: boolean; loadingOlder: boolean; loadOlder: () => void },
  /** The view is the reader's from here: the chat stops holding its bottom. */
  leave: () => void,
) {
  const want = aim.useValue();
  const pages = useRef(0);
  const { floor, hasOlder, loadingOlder, loadOlder } = chat;
  useEffect(() => {
    const box = stream.current;
    if (!want || want.agent !== agent || !box) return;
    const done = (said?: string) => {
      pages.current = 0;
      aim.set(null);
      if (said) toast(said, { tone: 'warning' });
    };
    if (Date.now() - want.asked > KEEP_MS) return done();
    if (show(box, want.message)) {
      leave();
      return done();
    }
    const step = aimStep({ floor, at: want.at, hasOlder, loadingOlder, pages: pages.current });
    if (step === 'gone') return done('That message is no longer in this chat.');
    if (step === 'far') return done('That message is too far back to show here.');
    if (step === 'wait') return;
    pages.current++;
    loadOlder();
  });
}
