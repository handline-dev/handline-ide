import { baseName } from '#src/format/path.ts';
import {
  CircleHelp,
  FileDiff,
  FileText,
  Globe,
  Gauge,
  House,
  Inbox,
  SlidersHorizontal,
  Settings,
  ListTodo,
  Workflow,
  MessageSquare,
  Plus,
  Smartphone,
  SquareTerminal,
  type LucideIcon,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { workspaceIcon } from '#src/workspace/icon.tsx';

/** Every kind of tab: its title and icon, shown alike in the tab bar, the + menu and an Empty tab's picker. */
const tab = (label: string, Icon: LucideIcon) => ({ label, icon: <Icon size={14} /> });
const kinds: Record<string, { label: string; icon: ReactNode }> = {
  chat: tab('Chat', MessageSquare),
  workspace: { label: 'Workspace', icon: workspaceIcon },
  shell: tab('Agent terminal', SquareTerminal),
  newshell: tab('Terminal', SquareTerminal),
  diff: tab('Changes', FileDiff),
  tasks: tab('Tasks', ListTodo),
  plan: tab('Plan', Workflow),
  markdown: tab('Markdown', FileText),
  web: tab('Web', Globe),
  android: tab('Android', Smartphone),
  ios: tab('iOS', Smartphone),
  requests: tab('Requests', Inbox),
  usage: tab('Usage', Gauge),
  town: tab('Town', House),
  settings: tab('Settings', Settings),
  preferences: tab('Settings', SlidersHorizontal),
  help: tab('Help', CircleHelp),
};

/** Another agent's chat, dragged in from the sidebar, opens `chat:<name>`. */
export const chatOf = (name: string) => `chat:${name}`;
/** In an agent's own workspace its chat is the pinned Chat tab, so `chat:<itself>` means `chat`. */
export const canonical = (opens: string, agent: string | null) => {
  if (agent && opens === chatOf(agent)) return 'chat';
  const own = ownedBy(opens);
  return agent && own?.agent === agent ? own.kind : opens;
};

/** Kinds that show one agent's thing (its terminal, a shell of its, its changes, its files). Outside that agent's workspace they carry
 * the agent: `<kind>|<agent>`, as a chat carries it in `chat:<agent>`. */
export const agentKind = (kind: string) => ['shell', 'newshell', 'diff', 'markdown'].includes(kind) || /^(shell|file):/.test(kind);
export const withAgent = (kind: string, agent: string) => `${kind}|${agent}`;
export const ownedBy = (opens: string) => {
  // | and a key with no slash: a file path or an address never reads as an agent.
  const i = opens.lastIndexOf('|');
  const agent = opens.slice(i + 1);
  return i > 0 && /^[\w.-]+$/.test(agent) && agentKind(opens.slice(0, i)) ? { kind: opens.slice(0, i), agent } : null;
};
/** A tab leaving `from`'s workspace keeps naming its agent. */
export const carry = (opens: string, from: string | null) => (from && agentKind(opens) && !ownedBy(opens) ? withAgent(opens, from) : opens);
const chatName = (kind: string) => (kind.startsWith('chat:') ? kind.slice(5) : null);

/** A plain shell shell.open opened (`shell:<handle>`); `newshell` is the menu's pick that opens one; `shell` alone is the agent's own terminal. */
export const shellOf = (handle: string) => `shell:${handle}`;
export const shellHandle = (kind: string) => (kind.startsWith('shell:') ? kind.slice(6) : null);

/** A tab on one thing, opened from a link: a task (`task:8503`), a page (`web:<address>`), a repo file (`file:<path>`). */
export const taskOf = (id: number) => `task:${id}`;
/** A task's sub-tasks as a graph (tools/Story): `plan:8503`. */
export const planOf = (id: number) => `plan:${id}`;
/** A task's changes, whole and per commit (tools/TaskCode): `code:8503`. */
export const codeOf = (id: number) => `code:${id}`;
export const webOf = (url: string) => `web:${url}`;
export const fileOf = (path: string) => `file:${path}`;
/** A text kept in this browser (tools/TextTab): `text:<when>/<name>`, titled by its name. */
export const textOf = (when: string, name: string) => `text:${when}/${name}`;
/** A Markdown file attached to a message or a task (tools/DocTab): `doc:<attachment id>/<name>`, titled by its name. */
export const docOf = (id: string, name: string) => `doc:${id}/${name}`;
/** An HR proposal's detail (requests/ProposalView): `proposal:41`. */
export const proposalOf = (id: number) => `proposal:${id}`;
/** A workspace's page. */
export const workspaceOf = (key: string) => `ws:${key}`;
export const opened = (kind: string, prefix: 'task' | 'plan' | 'code' | 'web' | 'file' | 'text' | 'doc' | 'proposal' | 'ws') =>
  kind.startsWith(`${prefix}:`) ? kind.slice(prefix.length + 1) : null;
const onThing = (kind: string): { label: string; icon: ReactNode } | null => {
  if (shellHandle(kind)) return kinds.newshell;
  if (opened(kind, 'ws')) return kinds.workspace;
  const task = opened(kind, 'task');
  if (task) return { label: `T${task}`, icon: kinds.tasks.icon };
  const plan = opened(kind, 'plan');
  if (plan) return { label: `Plan · T${plan}`, icon: kinds.plan.icon };
  const code = opened(kind, 'code');
  if (code) return { label: `T${code} Code`, icon: kinds.diff.icon };
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
  if (file) return { label: baseName(file), icon: kinds.markdown.icon };
  const hr = opened(kind, 'proposal');
  if (hr) return { label: `HR ${hr}`, icon: kinds.markdown.icon };
  const text = opened(kind, 'text') ?? opened(kind, 'doc');
  if (text) return { label: text.slice(text.indexOf('/') + 1), icon: kinds.markdown.icon };
  return null;
};

/** What a new tab can open (legacy CHOICES). Chat is not here: the first pane's Chat is the one chat. */
export const choices = ['newshell', 'diff', 'tasks', 'markdown', 'web', 'android', 'ios'].map((kind) => ({
  kind,
  ...kinds[kind],
}));

/** `nameOf` writes out the agent key a chat tab carries (useNameOf). */
export const labelOf = (kind: string, nameOf: (key: string) => string = (k) => k): string => {
  const own = ownedBy(kind);
  if (own) return `${labelOf(own.kind, nameOf)} · ${nameOf(own.agent)}`;
  const key = chatName(kind);
  const ws = opened(kind, 'ws');
  if (ws) return nameOf(ws);
  return kinds[kind]?.label ?? (key ? `${nameOf(key)} chat` : (onThing(kind)?.label ?? kind));
};
/** An Empty tab (no kind) shows +. */
export const iconOf = (kind?: string): ReactNode => {
  if (!kind) return <Plus size={14} />;
  const own = ownedBy(kind);
  if (own) return iconOf(own.kind);
  return kinds[kind]?.icon ?? (chatName(kind) ? kinds.chat.icon : (onThing(kind)?.icon ?? null));
};

/** The sidebar's pages: above the agents, and at the bottom (the app's settings and help). */
export const pages = ['tasks', 'requests', 'usage', 'town'].map((kind) => ({ kind, ...kinds[kind] }));
export const footerPages = ['preferences', 'help'].map((kind) => ({ kind, ...kinds[kind] }));
