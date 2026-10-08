import { describe, expect, it } from 'vitest';
import { formatGoDuration } from './format';

describe('formatGoDuration', () => {
  it('rounds single-unit Go durations to a readable unit', () => {
    expect(formatGoDuration('543.432µs')).toBe('543 µs');
    expect(formatGoDuration('1.2345ms')).toBe('1.2 ms');
    expect(formatGoDuration('25.6ms')).toBe('26 ms');
    expect(formatGoDuration('1500000ns')).toBe('1.5 ms');
    expect(formatGoDuration('2.5s')).toBe('2.5 s');
  });

  it('returns other durations unchanged', () => {
    expect(formatGoDuration('1m2.5s')).toBe('1m2.5s');
    expect(formatGoDuration('')).toBe('');
  });
});
