import {
  CircleHelp,
  FileDiff,
  FileText,
  Globe,
  Gauge,
  Inbox,
  SlidersHorizontal,
  Settings,
  ListTodo,
  MessageSquare,
  Plus,
  Smartphone,
  SquareTerminal,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { shortName } from '#src/layout/agentTree.ts';

/** Every kind of tab: its title and icon, shown alike in the tab bar, the + menu and an Empty tab's picker. */
const kinds: Record<string, { label: string; icon: ReactNode }> = {
  chat: { label: 'Chat', icon: <MessageSquare size={14} /> },
  shell: { label: 'Agent terminal', icon: <SquareTerminal size={14} /> },
  newshell: { label: 'Terminal', icon: <SquareTerminal size={14} /> },
  diff: { label: 'Changes', icon: <FileDiff size={14} /> },
  tasks: { label: 'Tasks', icon: <ListTodo size={14} /> },
  markdown: { label: 'Markdown', icon: <FileText size={14} /> },
  web: { label: 'Web', icon: <Globe size={14} /> },
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

/** A plain shell shell.open opened (`shell:<handle>`); `newshell` is the menu's pick that opens one; `shell` alone is the agent's own terminal. */
export const shellOf = (handle: string) => `shell:${handle}`;
export const shellHandle = (kind: string) => (kind.startsWith('shell:') ? kind.slice(6) : null);

/** A tab on one thing, opened from a link: a task (`task:8503`), a page (`web:<address>`), a repo file (`file:<path>`). */
export const taskOf = (id: number) => `task:${id}`;
export const webOf = (url: string) => `web:${url}`;
export const fileOf = (path: string) => `file:${path}`;
export const opened = (kind: string, prefix: 'task' | 'web' | 'file') =>
  kind.startsWith(`${prefix}:`) ? kind.slice(prefix.length + 1) : null;
const onThing = (kind: string): { label: string; icon: ReactNode } | null => {
  if (shellHandle(kind)) return kinds.newshell;
  const task = opened(kind, 'task');
  if (task) return { label: `T${task}`, icon: <ListTodo size={14} /> };
  const web = opened(kind, 'web');
  if (web) {
    let host = web;
    try {
      host = new URL(web).host;
    } catch {
      /* shown as typed */
    }
    return { label: host, icon: kinds.web.icon };
  }
  const file = opened(kind, 'file');
  if (file) return { label: file.split('/').pop() ?? file, icon: kinds.markdown.icon };
  return null;
};

/** What a new tab can open (legacy CHOICES). Chat is not here: the first pane's Chat is the one chat. */
export const choices = ['newshell', 'diff', 'tasks', 'markdown', 'web', 'android', 'ios'].map((kind) => ({ kind, ...kinds[kind] }));

/** `within`: the agent whose workspace shows it, so its own agents' chats read short (see shortName). */
export const labelOf = (kind: string, within: string | null = null) => {
  const name = chatName(kind);
  return kinds[kind]?.label ?? (name ? `${shortName(name, within)} chat` : (onThing(kind)?.label ?? kind));
};
/** An Empty tab (no kind) shows +. */
export const iconOf = (kind?: string) => {
  if (!kind) return <Plus size={14} />;
  return kinds[kind]?.icon ?? (chatName(kind) ? kinds.chat.icon : (onThing(kind)?.icon ?? null));
};

/** The sidebar's pages: above the agents, and at the bottom (the app's settings and help). */
export const pages = ['tasks', 'requests', 'usage'].map((kind) => ({ kind, ...kinds[kind] }));
export const footerPages = ['preferences', 'help'].map((kind) => ({ kind, ...kinds[kind] }));
