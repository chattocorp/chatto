import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';
import { timestampToDate, timestampToISO } from './timestamps';

describe('protobuf timestamp helpers', () => {
  it('converts a set timestamp', () => {
    const timestamp = timestampFromDate(new Date('2026-09-28T12:34:56.789Z'));
    expect(timestampToDate(timestamp)).toEqual(new Date('2026-09-28T12:34:56.789Z'));
    expect(timestampToISO(timestamp)).toBe('2026-09-28T12:34:56.789Z');
  });

  it('keeps an unset field unset', () => {
    expect(timestampToDate(undefined)).toBeUndefined();
    expect(timestampToISO(undefined)).toBeNull();
  });
});
