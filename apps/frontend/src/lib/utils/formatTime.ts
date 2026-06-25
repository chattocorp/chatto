/**
 * Centralized time formatting utilities that respect user settings.
 *
 * All functions accept either a Date object or an ISO string.
 * When timezone/timeFormat are unset, browser defaults are used.
 *
 * Intl.DateTimeFormat instances are cached because construction is expensive
 * (parses locale + timezone data). The cache is keyed by serialized options,
 * so formatters are reused across calls with the same settings.
 */

import type { UserSettingsState } from '$lib/state/userSettings.svelte';

const DAY_MS = 24 * 60 * 60 * 1000;

function toDate(date: Date | string): Date {
  return typeof date === 'string' ? new Date(date) : date;
}

/** Cache of Intl.DateTimeFormat instances keyed by locale + options. */
const formatterCache = new Map<string, Intl.DateTimeFormat>();

function getFormatter(
  locale: string | undefined,
  options: Intl.DateTimeFormatOptions
): Intl.DateTimeFormat {
  const key = `${locale ?? ''}:${JSON.stringify(options)}`;
  let fmt = formatterCache.get(key);
  if (!fmt) {
    fmt = new Intl.DateTimeFormat(locale, options);
    formatterCache.set(key, fmt);
  }
  return fmt;
}

type DateParts = {
  year: number;
  month: number;
  day: number;
};

type WeekInfo = {
  firstDay?: number;
};

type LocaleWithWeekInfo = {
  weekInfo?: WeekInfo;
  getWeekInfo?: () => WeekInfo;
};

export type FileDateGroup = {
  key: string;
  label: string;
};

function dateParts(date: Date, settings: UserSettingsState): DateParts {
  const fmt = getFormatter('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    timeZone: settings.effectiveTimezone
  });
  const parts = fmt.formatToParts(date);
  const value = (type: string) => Number(parts.find((part) => part.type === type)?.value ?? 0);
  return {
    year: value('year'),
    month: value('month'),
    day: value('day')
  };
}

function daySerial(parts: DateParts): number {
  return Math.floor(Date.UTC(parts.year, parts.month - 1, parts.day) / DAY_MS);
}

function weekday(parts: DateParts): number {
  return new Date(Date.UTC(parts.year, parts.month - 1, parts.day)).getUTCDay();
}

function normalizeFirstDay(firstDay: number | undefined): number | null {
  if (typeof firstDay !== 'number' || !Number.isInteger(firstDay)) return null;
  if (firstDay === 7) return 0;
  if (firstDay >= 1 && firstDay <= 6) return firstDay;
  if (firstDay === 0) return 0;
  return null;
}

export function firstDayOfWeekForLocale(locale?: string): number {
  const fallback = 1;
  const localeName =
    locale ?? globalThis.navigator?.languages?.[0] ?? globalThis.navigator?.language;
  if (!localeName || typeof Intl.Locale !== 'function') return fallback;

  try {
    const intlLocale = new Intl.Locale(localeName) as LocaleWithWeekInfo;
    const info = intlLocale.weekInfo ?? intlLocale.getWeekInfo?.();
    return normalizeFirstDay(info?.firstDay) ?? fallback;
  } catch {
    return fallback;
  }
}

function startOfWeekSerial(parts: DateParts, firstDay: number): number {
  const currentWeekday = weekday(parts);
  const offset = (currentWeekday - firstDay + 7) % 7;
  return daySerial(parts) - offset;
}

/**
 * Format a message timestamp (e.g., "2:30 PM" or "14:30").
 */
export function formatMessageTime(date: Date | string, settings: UserSettingsState): string {
  const fmt = getFormatter('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: settings.effectiveHour12,
    timeZone: settings.effectiveTimezone
  });
  return fmt.format(toDate(date));
}

/**
 * Format a date for display (e.g., "Jan 15, 2025").
 */
export function formatDate(date: Date | string, settings: UserSettingsState): string {
  const fmt = getFormatter(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    timeZone: settings.effectiveTimezone
  });
  return fmt.format(toDate(date));
}

/**
 * Format a date with time for display (e.g., "November 15, 2025, 02:30 PM").
 */
export function formatDateTime(date: Date | string, settings: UserSettingsState): string {
  const fmt = getFormatter(undefined, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: settings.effectiveHour12,
    timeZone: settings.effectiveTimezone
  });
  return fmt.format(toDate(date));
}

/**
 * Check if two dates fall on the same calendar day in the user's timezone.
 */
export function isSameDay(date1: Date, date2: Date, settings: UserSettingsState): boolean {
  const fmt = getFormatter('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    timeZone: settings.effectiveTimezone
  });
  return fmt.format(date1) === fmt.format(date2);
}

/**
 * Format a day separator label ("Today", "Yesterday", or a full date).
 */
export function formatDayLabel(date: Date | string, settings: UserSettingsState): string {
  const d = toDate(date);
  const now = new Date();

  if (isSameDay(d, now, settings)) {
    return 'Today';
  }

  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  if (isSameDay(d, yesterday, settings)) {
    return 'Yesterday';
  }

  const tz = settings.effectiveTimezone;
  const yearFmt = getFormatter('en-US', { year: 'numeric', timeZone: tz });
  const sameYear = yearFmt.format(d) === yearFmt.format(now);

  const labelFmt = getFormatter('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: sameYear ? undefined : 'numeric',
    timeZone: tz
  });
  return labelFmt.format(d);
}

export function formatMonthYear(date: Date | string, settings: UserSettingsState): string {
  const fmt = getFormatter(undefined, {
    month: 'long',
    year: 'numeric',
    timeZone: settings.effectiveTimezone
  });
  return fmt.format(toDate(date));
}

export function fileDateGroup(
  date: Date | string,
  settings: UserSettingsState,
  now: Date = new Date(),
  locale?: string
): FileDateGroup {
  const d = toDate(date);
  const itemParts = dateParts(d, settings);
  const nowParts = dateParts(now, settings);
  const daysAgo = daySerial(nowParts) - daySerial(itemParts);

  if (daysAgo === 0) return { key: 'today', label: 'Today' };
  if (daysAgo === 1) return { key: 'yesterday', label: 'Yesterday' };

  const firstDay = firstDayOfWeekForLocale(locale);
  if (startOfWeekSerial(itemParts, firstDay) === startOfWeekSerial(nowParts, firstDay)) {
    return { key: 'this-week', label: 'This week' };
  }

  if (itemParts.year === nowParts.year && itemParts.month === nowParts.month) {
    return { key: 'this-month', label: 'This month' };
  }

  return {
    key: `month:${itemParts.year}-${String(itemParts.month).padStart(2, '0')}`,
    label: formatMonthYear(d, settings)
  };
}
