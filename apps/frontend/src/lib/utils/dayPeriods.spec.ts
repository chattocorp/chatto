import { describe, expect, it } from 'vitest';
import { dayPeriodLabels } from './dayPeriods';

describe('dayPeriodLabels', () => {
  it('localizes the 12-hour clock periods', () => {
    expect(dayPeriodLabels('en-US')).toEqual({ am: 'AM', pm: 'PM' });
    expect(dayPeriodLabels('ja-JP')).toEqual({ am: '午前', pm: '午後' });
  });
});
