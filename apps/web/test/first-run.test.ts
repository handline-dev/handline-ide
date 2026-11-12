import { describe, expect, it } from 'vitest';
import { RpcError } from '@handline/client';
import { agentNameOf, factCards, notGit } from '#src/firstrun/FirstRun.tsx';

describe('agentNameOf', () => {
  it('names the agent for its folder', () => {
    expect(agentNameOf('My App_2')).toBe('my-app-2');
    expect(agentNameOf('handline-ide')).toBe('handline-ide');
    expect(agentNameOf('日本語')).toBe('agent');
  });
});

describe('factCards', () => {
  it('offers only what the repo has', () => {
    expect(factCards({ test_command: null, issues: null, todos: 0, uncommitted: 0 })).toEqual([]);
    expect(factCards(undefined)).toEqual([]);
    const cards = factCards({ test_command: 'pnpm test', issues: [{ number: 42, title: 'Checkout broken' }], todos: 3, uncommitted: 2 });
    expect(cards.map((c) => c.title)).toEqual([
      'Run the tests and fix what fails',
      '#42 Checkout broken',
      'Go through 3 TODOs',
      'Tidy 2 uncommitted changes',
    ]);
  });
});

describe('notGit', () => {
  it('is the facts error of a folder that is not a repository, and no other', () => {
    expect(notGit(new RpcError('not_found', '/x is not a git folder', { what: 'path', path: '/x' }))).toBe(true);
    expect(notGit(new RpcError('not_found', 'no agent', { what: 'agent' }))).toBe(false);
    expect(notGit(new RpcError('refused', 'no'))).toBe(false);
    expect(notGit(new Error('x'))).toBe(false);
  });
});
