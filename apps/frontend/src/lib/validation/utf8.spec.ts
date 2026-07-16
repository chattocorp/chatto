import { describe, expect, it } from 'vitest';
import { utf8ByteLength } from './utf8';

describe('UTF-8 validation', () => {
  it('counts UTF-8 bytes rather than JavaScript string length', () => {
    expect(utf8ByteLength('hello')).toBe(5);
    expect(utf8ByteLength('💬')).toBe(4);
  });
});
