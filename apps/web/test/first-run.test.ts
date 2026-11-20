import { describe, expect, it } from 'vitest';
import { factCards, workspaceNameOf } from '#src/onboarding/firstTask.ts';

describe('workspaceNameOf', () => {
  it('names the workspace for its folder, as workspace.create takes a name', () => {
    expect(workspaceNameOf('My App_2')).toBe('my-app-2');
    expect(workspaceNameOf('handline-ide')).toBe('handline-ide');
    expect(workspaceNameOf('2048-game')).toBe('game');
    expect(workspaceNameOf('a'.repeat(40))).toBe('a'.repeat(32));
    expect(workspaceNameOf('日本語')).toBe('project');
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
