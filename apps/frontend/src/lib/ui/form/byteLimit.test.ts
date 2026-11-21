import { describe, expect, it } from 'vitest';
import { fitToBytes } from './byteLimit';

describe('fitToBytes', () => {
  it('keeps text that fits', () => {
    expect(fitToBytes('💬ab', 6)).toBe('💬ab');
  });

  it('cuts at the end, counting UTF-8 bytes', () => {
    expect(fitToBytes('💬abc', 6)).toBe('💬ab');
  });

  it('never splits a character', () => {
    // The family emoji is one character of 25 bytes.
    expect(fitToBytes('ab👨‍👩‍👧‍👦', 26)).toBe('ab');
    expect(fitToBytes('😀😁', 5)).toBe('😀');
  });
});
