import { describe, expect, it } from 'vitest';
import { validatePassword } from './passwordPolicy.js';

describe('new password UTF-8 byte limits', () => {
  it.each([
    ['ASCII 7', 'a'.repeat(7), 'too_short'],
    ['ASCII 8', 'a'.repeat(8), undefined],
    ['ASCII 72', 'a'.repeat(72), undefined],
    ['ASCII 73', 'a'.repeat(73), 'too_long'],
    ['ASCII 128', 'a'.repeat(128), 'too_long'],
    ['multibyte 8', '😀'.repeat(2), undefined],
    ['multibyte 72', '😀'.repeat(18), undefined],
    ['multibyte 73', '😀'.repeat(18) + 'a', 'too_long'],
    ['multibyte 128', '😀'.repeat(32), 'too_long']
  ])('%s', (_name, password, expected) => {
    expect(validatePassword(password!)).toBe(expected);
  });
});
