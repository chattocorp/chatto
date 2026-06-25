import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  fileDateGroup,
  firstDayOfWeekForLocale,
  formatMessageTime,
  formatDate,
  formatDateTime,
  formatMonthYear,
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

describe('formatMonthYear', () => {
  it('formats the month and year in the user timezone', () => {
    const la = settings('America/Los_Angeles', false);
    expect(formatMonthYear('2026-06-01T01:00:00Z', la)).toMatch(/May\s*2026/);
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

describe('firstDayOfWeekForLocale', () => {
  it('uses locale week information when available', () => {
    expect(firstDayOfWeekForLocale('en-US')).toBe(0);
    expect(firstDayOfWeekForLocale('de-DE')).toBe(1);
  });

  it('falls back to Monday for invalid locale input', () => {
    expect(firstDayOfWeekForLocale('invalid_locale')).toBe(1);
  });

  it('uses the browser locale when no explicit locale is passed', () => {
    vi.stubGlobal('navigator', { languages: ['en-US'], language: 'en-US' });

    try {
      expect(firstDayOfWeekForLocale()).toBe(0);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe('fileDateGroup', () => {
  const now = new Date('2026-06-17T12:00:00Z');

  it('groups files from today', () => {
    expect(fileDateGroup('2026-06-17T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'today',
      label: 'Today'
    });
  });

  it('groups files from yesterday', () => {
    expect(fileDateGroup('2026-06-16T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'yesterday',
      label: 'Yesterday'
    });
  });

  it('groups earlier files in the current locale week', () => {
    expect(fileDateGroup('2026-06-15T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'this-week',
      label: 'This week'
    });
  });

  it('uses the locale week start for this-week grouping', () => {
    expect(fileDateGroup('2026-06-14T08:00:00Z', utc12, now, 'en-US')).toEqual({
      key: 'this-week',
      label: 'This week'
    });

    expect(fileDateGroup('2026-06-14T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'this-month',
      label: 'This month'
    });
  });

  it('groups files from earlier in the current month', () => {
    expect(fileDateGroup('2026-06-10T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'this-month',
      label: 'This month'
    });
  });

  it('lets this week take precedence across a month boundary', () => {
    expect(
      fileDateGroup(
        '2026-05-31T08:00:00Z',
        utc12,
        new Date('2026-06-03T12:00:00Z'),
        'en-US'
      )
    ).toEqual({
      key: 'this-week',
      label: 'This week'
    });
  });

  it('groups older files by calendar month and year', () => {
    expect(fileDateGroup('2026-05-21T08:00:00Z', utc12, now, 'de-DE')).toEqual({
      key: 'month:2026-05',
      label: 'May 2026'
    });
  });

  it('uses the user timezone for calendar-day boundaries', () => {
    const berlin = settings('Europe/Berlin', false);

    expect(
      fileDateGroup(
        '2026-06-16T23:30:00Z',
        berlin,
        new Date('2026-06-17T00:30:00Z'),
        'de-DE'
      )
    ).toEqual({
      key: 'today',
      label: 'Today'
    });
  });
});

describe('formatter cache', () => {
  it('produces consistent output across calls (cache hit path)', () => {
    const a = formatMessageTime('2025-04-27T14:30:00Z', utc12);
    const b = formatMessageTime('2025-04-27T14:30:00Z', utc12);
    expect(a).toBe(b);
  });
});
