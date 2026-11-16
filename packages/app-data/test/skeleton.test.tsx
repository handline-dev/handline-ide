import { describe, expect, it } from 'vitest';
import { SKELETON_CSS } from '#src/index.ts';

describe('the skeleton look', () => {
  it('the shimmer stops when the system asks for reduced motion', () => {
    expect(SKELETON_CSS).toMatch(/@media \(prefers-reduced-motion: reduce\) \{ \.sk \{ animation: none; \} \}/);
    expect(SKELETON_CSS).not.toMatch(/spin|rotate/);
  });
});
