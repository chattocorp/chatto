import { describe, expect, it, vi } from 'vitest';
import {
  formatSlowModeCountdown,
  formatSlowModeInterval,
  slowModeRemainingSeconds,
  SLOW_MODE_PRESETS
} from './slowMode';

describe('Slow Mode formatting', () => {
  it('exposes the supported intervals through the six-hour maximum', () => {
    expect(SLOW_MODE_PRESETS).toEqual([
      0, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 21600
    ]);
  });

  it('formats preset intervals with locale-aware units', () => {
    expect(formatSlowModeInterval(30, 'en-GB')).toBe('30 seconds');
    expect(formatSlowModeInterval(120, 'en-GB')).toBe('2 minutes');
    expect(formatSlowModeInterval(21600, 'en-GB')).toBe('6 hours');
  });

  it('rounds countdowns up and switches to hours when necessary', () => {
    expect(formatSlowModeCountdown(0.1)).toBe('0:01');
    expect(formatSlowModeCountdown(65)).toBe('1:05');
    expect(formatSlowModeCountdown(3661)).toBe('1:01:01');
  });

  it('expires the deadline at an exact boundary', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-08-11T12:00:00.000Z'));
    const deadline = Date.now() + 1_500;
    expect(slowModeRemainingSeconds(deadline)).toBe(2);
    vi.advanceTimersByTime(1_500);
    expect(slowModeRemainingSeconds(deadline)).toBe(0);
    vi.useRealTimers();
  });
});
