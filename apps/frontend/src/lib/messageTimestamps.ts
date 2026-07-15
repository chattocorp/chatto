import { parseTrustedMarkdownHtml } from '$lib/security/trustedHtml';
import { formatDateTime, type TimeFormatSettings } from '$lib/utils/formatTime';

const TOKEN_REGEX = /<t:(\d{1,12}):F>/g;
const MAX_UNIX_SECONDS = Math.floor(8.64e15 / 1000);
const EXCLUDED_ELEMENTS = ['PRE', 'CODE', 'BLOCKQUOTE', 'A', 'BUTTON'];
const RELATIVE_UNITS = [
  ['year', 365 * 24 * 60 * 60],
  ['month', 30 * 24 * 60 * 60],
  ['week', 7 * 24 * 60 * 60],
  ['day', 24 * 60 * 60],
  ['hour', 60 * 60],
  ['minute', 60],
  ['second', 1]
] as const;

export type MessageTimestampToken = {
  epochSeconds: number;
  format: 'F';
};

type DateTimeParts = {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
};

function isInsideExcludedElement(node: Node): boolean {
  let current: Node | null = node.parentNode;
  while (current && current.nodeType === Node.ELEMENT_NODE) {
    if (EXCLUDED_ELEMENTS.includes((current as Element).tagName)) return true;
    current = current.parentNode;
  }
  return false;
}

function timestampTokenText(epochSeconds: number): string {
  return `<t:${epochSeconds}:F>`;
}

function parseTimestampParts(localValue: string): DateTimeParts | null {
  const match = localValue.match(
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/
  );
  if (!match) return null;

  const [, year, month, day, hour, minute, second = '0'] = match;
  const parts = {
    year: Number(year),
    month: Number(month),
    day: Number(day),
    hour: Number(hour),
    minute: Number(minute),
    second: Number(second)
  };
  if (
    parts.month < 1 ||
    parts.month > 12 ||
    parts.day < 1 ||
    parts.day > 31 ||
    parts.hour > 23 ||
    parts.minute > 59 ||
    parts.second > 59
  ) {
    return null;
  }
  return parts;
}

function numericPart(parts: Intl.DateTimeFormatPart[], type: string): number {
  return Number(parts.find((part) => part.type === type)?.value ?? 0);
}

function dateTimePartsInZone(date: Date, timeZone: string): DateTimeParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  }).formatToParts(date);

  return {
    year: numericPart(parts, 'year'),
    month: numericPart(parts, 'month'),
    day: numericPart(parts, 'day'),
    hour: numericPart(parts, 'hour'),
    minute: numericPart(parts, 'minute'),
    second: numericPart(parts, 'second')
  };
}

function timeZoneOffsetMs(date: Date, timeZone: string): number {
  const parts = dateTimePartsInZone(date, timeZone);
  const zonedAsUTC = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second
  );
  return zonedAsUTC - date.getTime();
}

function sameWallTime(a: DateTimeParts, b: DateTimeParts): boolean {
  return (
    a.year === b.year &&
    a.month === b.month &&
    a.day === b.day &&
    a.hour === b.hour &&
    a.minute === b.minute &&
    a.second === b.second
  );
}

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

export function parseMessageTimestampToken(value: string): MessageTimestampToken | null {
  const match = value.match(/^<t:(\d{1,12}):F>$/);
  if (!match) return null;
  const epochSeconds = Number(match[1]);
  if (!Number.isSafeInteger(epochSeconds) || epochSeconds < 0 || epochSeconds > MAX_UNIX_SECONDS) {
    return null;
  }
  return { epochSeconds, format: 'F' };
}

export function createMessageTimestampToken(epochSeconds: number): string {
  if (
    !Number.isSafeInteger(epochSeconds) ||
    epochSeconds < 0 ||
    epochSeconds > MAX_UNIX_SECONDS
  ) {
    throw new RangeError('Timestamp is outside the supported Unix seconds range');
  }
  return timestampTokenText(epochSeconds);
}

export function dateToDatetimeLocalValue(date: Date, timeZone: string): string {
  const parts = dateTimePartsInZone(date, timeZone);
  return `${parts.year}-${pad(parts.month)}-${pad(parts.day)}T${pad(parts.hour)}:${pad(parts.minute)}`;
}

export function localDatetimeToEpochSeconds(localValue: string, timeZone: string): number | null {
  const parts = parseTimestampParts(localValue);
  if (!parts || !timeZone) return null;

  const localAsUTC = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second
  );
  let utcMs = localAsUTC - timeZoneOffsetMs(new Date(localAsUTC), timeZone);
  utcMs = localAsUTC - timeZoneOffsetMs(new Date(utcMs), timeZone);

  const date = new Date(utcMs);
  if (Number.isNaN(date.getTime()) || !sameWallTime(parts, dateTimePartsInZone(date, timeZone))) {
    return null;
  }

  return Math.floor(utcMs / 1000);
}

export function formatRelativeMessageTimestamp(
  date: Date,
  locale: string,
  now = new Date()
): string {
  const diffSeconds = Math.round((date.getTime() - now.getTime()) / 1000);
  const absoluteDiffSeconds = Math.abs(diffSeconds);
  const [unit, unitSeconds] =
    RELATIVE_UNITS.find(([, seconds]) => absoluteDiffSeconds >= seconds) ??
    RELATIVE_UNITS[RELATIVE_UNITS.length - 1];
  const value = Math.round(diffSeconds / unitSeconds);
  return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(value, unit);
}

export function wrapMessageTimestamps(
  html: string,
  settings: TimeFormatSettings,
  locale: string
): string {
  if (!html.includes('&lt;t:') && !html.includes('<t:')) return html;

  const doc = parseTrustedMarkdownHtml(html);
  const textNodes: Text[] = [];
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT);

  let node: Text | null;
  while ((node = walker.nextNode() as Text | null)) {
    if (!isInsideExcludedElement(node)) textNodes.push(node);
  }

  for (const textNode of textNodes) {
    const text = textNode.textContent ?? '';
    if (!text.includes('<t:')) continue;

    const fragments: (string | Element)[] = [];
    let lastIndex = 0;
    let match: RegExpExecArray | null;
    TOKEN_REGEX.lastIndex = 0;

    while ((match = TOKEN_REGEX.exec(text)) !== null) {
      const token = parseMessageTimestampToken(match[0]);
      if (!token) continue;

      if (match.index > lastIndex) fragments.push(text.slice(lastIndex, match.index));

      const date = new Date(token.epochSeconds * 1000);
      const iso = date.toISOString();
      const rendered = formatDateTime(date, settings, locale);
      const button = doc.createElement('button');
      button.type = 'button';
      button.className = 'message-timestamp';
      button.setAttribute('data-timestamp-epoch', String(token.epochSeconds));
      button.setAttribute('aria-haspopup', 'dialog');

      const icon = doc.createElement('span');
      icon.className = 'message-timestamp-icon iconify uil--clock';
      icon.setAttribute('aria-hidden', 'true');

      const time = doc.createElement('time');
      time.setAttribute('datetime', iso);
      time.setAttribute('title', iso);
      time.textContent = rendered;

      button.appendChild(icon);
      button.appendChild(time);
      fragments.push(button);
      lastIndex = match.index + match[0].length;
    }

    if (lastIndex === 0) continue;
    if (lastIndex < text.length) fragments.push(text.slice(lastIndex));

    const parent = textNode.parentNode;
    if (!parent) continue;
    for (const fragment of fragments) {
      parent.insertBefore(
        typeof fragment === 'string' ? doc.createTextNode(fragment) : fragment,
        textNode
      );
    }
    parent.removeChild(textNode);
  }

  return doc.body.innerHTML;
}
