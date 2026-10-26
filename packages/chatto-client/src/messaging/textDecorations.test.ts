import { expect, it } from 'vitest';
import { isTextDecoration, TEXT_DECORATIONS } from './textDecorations.js';

it('accepts only the documented attribute-free tag names', () => {
  for (const tag of TEXT_DECORATIONS) expect(isTextDecoration(tag)).toBe(true);
  for (const tag of ['script', 'color-blue onclick=x', 'COLOR-BLUE', '__proto__', '']) {
    expect(isTextDecoration(tag)).toBe(false);
  }
});
