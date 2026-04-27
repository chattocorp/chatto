import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  formatMessageTime,
  formatDate,
  formatDateTime,
  formatDayLabel,
  isSameDay
} from './formatTime';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';

function settings(timezone: string | undefined, hour12?: boolean): UserSettingsState {
  return {
    get effectiveTimezone() {
      return timezone;
    },
    get effectiveHour12() {
      return hour12;
    }
  } as unknown as UserSettingsState;
}

const utc12 = settings('UTC', false);
const utc12h = settings('UTC', true);

describe('formatMessageTime', () => {
  it('formats UTC time in 24-hour format', () => {
    expect(formatMessageTime('2025-04-27T14:30:00Z', utc12)).toBe('14:30');
  });

  it('formats UTC time in 12-hour format with AM/PM', () => {
    expect(formatMessageTime('2025-04-27T14:30:00Z', utc12h)).toBe('02:30 PM');
  });

  it('respects timezone offsets', () => {
    const tokyo = settings('Asia/Tokyo', false);
    expect(formatMessageTime('2025-04-27T14:30:00Z', tokyo)).toBe('23:30');
  });

  it('accepts a Date object', () => {
    expect(formatMessageTime(new Date('2025-04-27T14:30:00Z'), utc12)).toBe('14:30');
  });
});

describe('formatDate', () => {
  it('formats a date in long-month/short style', () => {
    expect(formatDate('2025-04-27T14:30:00Z', utc12)).toMatch(/Apr\s*27,?\s*2025/);
  });

  it('crosses midnight in non-UTC zone correctly', () => {
    // 2025-04-27T01:00Z is still 2025-04-26 in Los Angeles
    const la = settings('America/Los_Angeles', false);
    expect(formatDate('2025-04-27T01:00:00Z', la)).toMatch(/Apr\s*26/);
  });
});

describe('formatDateTime', () => {
  it('includes both date and time', () => {
    const out = formatDateTime('2025-04-27T14:30:00Z', utc12);
    expect(out).toMatch(/April\s*27,?\s*2025/);
    expect(out).toContain('14:30');
  });
});

describe('isSameDay', () => {
  it('returns true for same UTC day', () => {
    expect(
      isSameDay(new Date('2025-04-27T01:00:00Z'), new Date('2025-04-27T23:00:00Z'), utc12)
    ).toBe(true);
  });

  it('returns false for different UTC days', () => {
    expect(
      isSameDay(new Date('2025-04-27T23:00:00Z'), new Date('2025-04-28T01:00:00Z'), utc12)
    ).toBe(false);
  });

  it('is timezone-aware: same local day across UTC midnight', () => {
    const la = settings('America/Los_Angeles', false);
    // 2025-04-27T05:00Z = 2025-04-26 22:00 LA, 2025-04-27T07:00Z = 2025-04-27 00:00 LA
    // Different local day in LA
    expect(
      isSameDay(new Date('2025-04-27T05:00:00Z'), new Date('2025-04-27T07:00:00Z'), la)
    ).toBe(false);
  });
});

describe('formatDayLabel', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-04-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "Today" for today', () => {
    expect(formatDayLabel('2025-04-27T08:00:00Z', utc12)).toBe('Today');
  });

  it('returns "Yesterday" for yesterday', () => {
    expect(formatDayLabel('2025-04-26T08:00:00Z', utc12)).toBe('Yesterday');
  });

  it('returns weekday + month + day for same year, older than yesterday', () => {
    const out = formatDayLabel('2025-03-10T12:00:00Z', utc12);
    expect(out).toMatch(/Monday/);
    expect(out).toMatch(/March/);
    expect(out).toMatch(/10/);
    expect(out).not.toMatch(/2025/);
  });

  it('includes year for prior years', () => {
    const out = formatDayLabel('2024-03-10T12:00:00Z', utc12);
    expect(out).toMatch(/March/);
    expect(out).toMatch(/2024/);
  });
});

describe('formatter cache', () => {
  it('produces consistent output across calls (cache hit path)', () => {
    const a = formatMessageTime('2025-04-27T14:30:00Z', utc12);
    const b = formatMessageTime('2025-04-27T14:30:00Z', utc12);
    expect(a).toBe(b);
  });
});
