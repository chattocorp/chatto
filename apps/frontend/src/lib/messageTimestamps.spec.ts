import { describe, expect, it } from 'vitest';
import {
  dateToDatetimeLocalValue,
  formatRelativeMessageTimestamp,
  localDatetimeToEpochSeconds
} from './messageTimestamps';

describe('message timestamp formatting', () => {
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
