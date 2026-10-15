import '../../../app.css';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import ComposerTimestampPicker from './ComposerTimestampPicker.svelte';

describe('timestamp picker defaults', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-10-15T22:37:49Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('opens at the current minute in the preferred timezone', async () => {
    render(ComposerTimestampPicker, {
      props: { disabled: false, editorApi: null, effectiveTimezone: 'Europe/Berlin' }
    });

    await page.getByRole('button', { name: 'Insert timestamp', exact: true }).click();

    await expect.element(page.getByLabelText('Date and time')).toHaveValue('2026-10-16T00:37');
    await expect
      .element(page.getByLabelText('Timezone', { exact: true }))
      .toHaveValue('Europe/Berlin');
  });

  it('uses the browser timezone when no preference is set', async () => {
    render(ComposerTimestampPicker, { props: { disabled: false, editorApi: null } });
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23'
    }).formatToParts(new Date());
    const part = (type: Intl.DateTimeFormatPartTypes) =>
      parts.find((entry) => entry.type === type)?.value;
    const expected = `${part('year')}-${part('month')}-${part('day')}T${part('hour')}:${part('minute')}`;

    await page.getByRole('button', { name: 'Insert timestamp', exact: true }).click();

    await expect.element(page.getByLabelText('Date and time')).toHaveValue(expected);
    await expect.element(page.getByLabelText('Timezone', { exact: true })).toHaveValue(timezone);
  });

  it('reads the current time again when reopened', async () => {
    render(ComposerTimestampPicker, {
      props: { disabled: false, editorApi: null, effectiveTimezone: 'Europe/Berlin' }
    });
    await page.getByRole('button', { name: 'Insert timestamp', exact: true }).click();
    await expect.element(page.getByLabelText('Date and time')).toHaveValue('2026-10-16T00:37');
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();

    vi.setSystemTime(new Date('2026-10-15T22:42:03Z'));
    await page.getByRole('button', { name: 'Insert timestamp', exact: true }).click();

    await expect.element(page.getByLabelText('Date and time')).toHaveValue('2026-10-16T00:42');
  });
});
