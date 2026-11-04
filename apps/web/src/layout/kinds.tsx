import { CircleHelp, Gauge, Inbox, SlidersHorizontal, Settings, ListTodo, MessageSquare, Plus, Smartphone, SquareTerminal } from 'lucide-react';
import type { ReactNode } from 'react';
import { shortName } from '#src/layout/agentTree.ts';

/** Every kind of tab: its title and icon, shown alike in the tab bar, the + menu and an Empty tab's picker. */
const kinds: Record<string, { label: string; icon: ReactNode }> = {
  chat: { label: 'Chat', icon: <MessageSquare size={14} /> },
  shell: { label: 'Terminal', icon: <SquareTerminal size={14} /> },
  tasks: { label: 'Tasks', icon: <ListTodo size={14} /> },
  android: { label: 'Android', icon: <Smartphone size={14} /> },
  ios: { label: 'iOS', icon: <Smartphone size={14} /> },
  requests: { label: 'Requests', icon: <Inbox size={14} /> },
  usage: { label: 'Usage', icon: <Gauge size={14} /> },
  settings: { label: 'Settings', icon: <Settings size={14} /> },
  preferences: { label: 'Settings', icon: <SlidersHorizontal size={14} /> },
  help: { label: 'Help', icon: <CircleHelp size={14} /> },
};

/** Another agent's chat, dragged in from the sidebar, opens `chat:<name>`. */
export const chatOf = (name: string) => `chat:${name}`;
/** In an agent's own workspace its chat is the pinned Chat tab, so `chat:<itself>` means `chat`. */
export const canonical = (opens: string, agent: string | null) => (agent && opens === chatOf(agent) ? 'chat' : opens);
const chatName = (kind: string) => (kind.startsWith('chat:') ? kind.slice(5) : null);

/** What a new tab can open (legacy CHOICES). Chat is not here: the first pane's Chat is the one chat. */
export const choices = ['shell', 'tasks', 'android', 'ios'].map((kind) => ({ kind, ...kinds[kind] }));

/** `within`: the agent whose workspace shows it, so its own agents' chats read short (see shortName). */
export const labelOf = (kind: string, within: string | null = null) => {
  const name = chatName(kind);
  return kinds[kind]?.label ?? (name ? `${shortName(name, within)} chat` : kind);
};
/** An Empty tab (no kind) shows +. */
export const iconOf = (kind?: string) => {
  if (!kind) return <Plus size={14} />;
  return kinds[kind]?.icon ?? (chatName(kind) ? kinds.chat.icon : null);
};

/** The sidebar's pages: above the agents, and at the bottom (the app's settings and help). */
export const pages = ['tasks', 'requests', 'usage'].map((kind) => ({ kind, ...kinds[kind] }));
export const footerPages = ['preferences', 'help'].map((kind) => ({ kind, ...kinds[kind] }));
