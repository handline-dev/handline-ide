import { describe, expect, it } from 'vitest';
import { agentNameOf } from '#src/firstrun/FirstRun.tsx';

describe('agentNameOf', () => {
  it('names the agent for its folder', () => {
    expect(agentNameOf('My App_2')).toBe('my-app-2');
    expect(agentNameOf('handline-ide')).toBe('handline-ide');
    expect(agentNameOf('日本語')).toBe('agent');
  });
});
