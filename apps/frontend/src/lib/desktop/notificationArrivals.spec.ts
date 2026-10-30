import { describe, expect, it } from 'vitest';
import { NotificationArrivals } from './notificationArrivals';

const row = (id: string, seconds: number) => ({
  id,
  createdAt: new Date(seconds * 1000).toISOString()
});
describe('NotificationArrivals', () => {
  it('baselines old notifications, then detects polled arrivals without duplicates', () => {
    const arrivals = new NotificationArrivals();
    const old = row('old', 100);
    const fresh = row('new', 101);
    expect(arrivals.read([old])).toEqual([]);
    expect(arrivals.read([fresh, old])).toEqual([fresh]);
    expect(arrivals.read([fresh, old])).toEqual([]);
    expect(arrivals.read([fresh, old, row('older-page', 99)])).toEqual([]);
  });
  it('handles equal timestamps and empty reads without re-alerting', () => {
    const arrivals = new NotificationArrivals();
    arrivals.read([row('one', 100)]);
    expect(arrivals.read([row('two', 100)])).toEqual([row('two', 100)]);
    arrivals.read([]);
    expect(arrivals.read([row('one', 100), row('two', 100)])).toEqual([]);
  });
  it('takes a fresh baseline after a privacy reset', () => {
    const arrivals = new NotificationArrivals();
    arrivals.read([]);
    arrivals.reset();
    expect(arrivals.read([row('other-account', 200)])).toEqual([]);
    expect(arrivals.read([row('fresh', 201)])).toEqual([row('fresh', 201)]);
  });
});
