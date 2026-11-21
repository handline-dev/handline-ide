import { expect, test } from 'vitest';
import { nameOf } from '#src/NewWorkspace.tsx';

test('a workspace is named after its folder, as onboarding names it', () => {
  expect(nameOf('/Users/me/workspaces/My Shop')).toBe('my-shop');
  expect(nameOf('/Users/founder/Handline/project/')).toBe('project');
});
