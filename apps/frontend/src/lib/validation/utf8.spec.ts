import { describe, expect, it } from 'vitest';
import { truncateUtf8, utf8ByteLength } from './utf8';

describe('UTF-8 validation', () => {
  it('counts UTF-8 bytes rather than JavaScript string length', () => {
    expect(utf8ByteLength('hello')).toBe(5);
    expect(utf8ByteLength('💬')).toBe(4);
  });

  it('keeps the longest complete-code-point prefix within the byte limit', () => {
    expect(truncateUtf8('a'.repeat(501), 500)).toBe('a'.repeat(500));
    expect(truncateUtf8(`${'💬'.repeat(125)}a💬`, 500)).toBe('💬'.repeat(125));
  });
});
