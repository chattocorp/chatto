import { describe, expect, it } from 'vitest';
import {
  createMessageTimestampToken,
  dateToDatetimeLocalValue,
  formatRelativeMessageTimestamp,
  localDatetimeToEpochSeconds,
  parseMessageTimestampToken
} from './messageTimestamps';

describe('message timestamp tokens', () => {
  it('creates and parses exact timestamp tokens', () => {
    expect(createMessageTimestampToken(1745764200)).toBe('<t:1745764200:F>');
    expect(parseMessageTimestampToken('<t:1745764200:F>')).toEqual({
      epochSeconds: 1745764200,
      format: 'F'
    });
  });

  it('rejects unsupported token formats', () => {
    expect(parseMessageTimestampToken('<t:1745764200:R>')).toBeNull();
    expect(parseMessageTimestampToken('<t:abc:F>')).toBeNull();
  });

  it('converts a zoned local date-time to Unix seconds', () => {
    expect(localDatetimeToEpochSeconds('2025-04-27T14:30', 'UTC')).toBe(1745764200);
    expect(localDatetimeToEpochSeconds('2025-04-27T16:30', 'Europe/Berlin')).toBe(1745764200);
  });

  it('formats a date as a datetime-local value in the requested timezone', () => {
    expect(dateToDatetimeLocalValue(new Date('2025-04-27T14:30:00Z'), 'Europe/Berlin')).toBe(
      '2025-04-27T16:30'
    );
  });

  it('formats timestamps relative to the current time', () => {
    expect(
      formatRelativeMessageTimestamp(
        new Date('2025-04-27T14:30:00Z'),
        'en-US',
        new Date('2025-04-27T13:30:00Z')
      )
    ).toBe('in 1 hour');
  });
});
