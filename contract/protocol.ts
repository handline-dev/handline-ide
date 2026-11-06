// Generated from contract/spec/ by contract/generate.py; do not edit.
/* eslint-disable */
import type { Role } from './gen/core.ts';
import { CORE_METHODS, CORE_APP_CACHE, CORE_APP_WRITES, type CoreMethods } from './gen/core.ts';
import { ACCOUNTS_METHODS, ACCOUNTS_APP_CACHE, ACCOUNTS_APP_WRITES, type AccountsMethods } from './gen/accounts.ts';
import { AGENTS_METHODS, AGENTS_APP_CACHE, AGENTS_APP_WRITES, type AgentsMethods } from './gen/agents.ts';
import { CHAT_METHODS, CHAT_APP_CACHE, CHAT_APP_WRITES, type ChatMethods } from './gen/chat.ts';
import { DEVICES_METHODS, DEVICES_APP_CACHE, DEVICES_APP_WRITES, type DevicesMethods } from './gen/devices.ts';
import { REPOS_METHODS, REPOS_APP_CACHE, REPOS_APP_WRITES, type ReposMethods } from './gen/repos.ts';
import { TASKS_METHODS, TASKS_APP_CACHE, TASKS_APP_WRITES, type TasksMethods } from './gen/tasks.ts';
import { TEAM_METHODS, TEAM_APP_CACHE, TEAM_APP_WRITES, type TeamMethods } from './gen/team.ts';
import { TERMINAL_METHODS, TERMINAL_APP_CACHE, TERMINAL_APP_WRITES, type TerminalMethods } from './gen/terminal.ts';

export * from './gen/core.ts';
export * from './gen/accounts.ts';
export * from './gen/agents.ts';
export * from './gen/chat.ts';
export * from './gen/devices.ts';
export * from './gen/repos.ts';
export * from './gen/tasks.ts';
export * from './gen/team.ts';
export * from './gen/terminal.ts';

export interface Methods extends CoreMethods, AccountsMethods, AgentsMethods, ChatMethods, DevicesMethods, ReposMethods, TasksMethods, TeamMethods, TerminalMethods {}

export type MethodName = keyof Methods;

export const METHODS: { [K in MethodName]: { role: Role; callers: Role[] | null; write: boolean; since: number } } = {
  ...CORE_METHODS,
  ...ACCOUNTS_METHODS,
  ...AGENTS_METHODS,
  ...CHAT_METHODS,
  ...DEVICES_METHODS,
  ...REPOS_METHODS,
  ...TASKS_METHODS,
  ...TEAM_METHODS,
  ...TERMINAL_METHODS,
};

/** How the app caches each read and writes each write (x-app in core.json). */
export const APP_CACHE = {
  ...CORE_APP_CACHE,
  ...ACCOUNTS_APP_CACHE,
  ...AGENTS_APP_CACHE,
  ...CHAT_APP_CACHE,
  ...DEVICES_APP_CACHE,
  ...REPOS_APP_CACHE,
  ...TASKS_APP_CACHE,
  ...TEAM_APP_CACHE,
  ...TERMINAL_APP_CACHE,
};

export const APP_WRITES = {
  ...CORE_APP_WRITES,
  ...ACCOUNTS_APP_WRITES,
  ...AGENTS_APP_WRITES,
  ...CHAT_APP_WRITES,
  ...DEVICES_APP_WRITES,
  ...REPOS_APP_WRITES,
  ...TASKS_APP_WRITES,
  ...TEAM_APP_WRITES,
  ...TERMINAL_APP_WRITES,
};
