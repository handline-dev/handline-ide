import { describe, expect, it } from 'vitest';
import { agentNameOf, factCards } from '#src/onboarding/firstTask.ts';

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
