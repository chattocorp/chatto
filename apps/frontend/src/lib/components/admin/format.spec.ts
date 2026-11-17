import { describe, expect, it } from 'vitest';
import { formatGoDuration, formatSeconds } from './format';

describe('formatGoDuration', () => {
  it('rounds single-unit Go durations to a readable unit', () => {
    expect(formatGoDuration('543.432µs')).toBe('543 µs');
    expect(formatGoDuration('1.2345ms')).toBe('1.2 ms');
    expect(formatGoDuration('25.6ms')).toBe('26 ms');
    expect(formatGoDuration('1500000ns')).toBe('1.5 ms');
    expect(formatGoDuration('2.5s')).toBe('2.5 s');
  });

  it('switches to the larger unit when rounding reaches it', () => {
    expect(formatGoDuration('999.9µs')).toBe('1 ms');
    expect(formatGoDuration('999.6ms')).toBe('1 s');
  });

  it('returns other durations unchanged', () => {
    expect(formatGoDuration('1m2.5s')).toBe('1m2.5s');
    expect(formatGoDuration('')).toBe('');
  });
});

describe('formatSeconds', () => {
  it('chooses a readable unit', () => {
    expect(formatSeconds(0.0001)).toBe('<1 ms');
    expect(formatSeconds(0.25)).toBe('250 ms');
    expect(formatSeconds(4.2)).toBe('4.20 s');
    expect(formatSeconds(12.34)).toBe('12.3 s');
    expect(formatSeconds(400)).toBe('6m 40s');
  });
});
