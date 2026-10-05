import '../../../app.css';
import { describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import TimeInputTestHarness from './TimeInputTestHarness.svelte';

function renderTime(props: { value?: string; hour12: boolean }) {
  const { container } = render(TimeInputTestHarness, { props });
  return {
    hour: container.querySelector('input[aria-label="Hour"]') as HTMLInputElement,
    minute: container.querySelector('input[aria-label="Minute"]') as HTMLInputElement,
    period: () =>
      container.querySelector('select[aria-label="AM or PM"]') as HTMLSelectElement | null,
    bound: () => container.querySelector('output')!.textContent,
    setValue: () => (container.querySelector('button') as HTMLButtonElement).click()
  };
}

describe('TimeInput', () => {
  it('shows a 24-hour clock without a day period', () => {
    const field = renderTime({ value: '14:30', hour12: false });

    expect(field.hour.value).toBe('14');
    expect(field.minute.value).toBe('30');
    expect(field.period()).toBeNull();
  });

  it('shows a 12-hour clock with a day period', () => {
    const field = renderTime({ value: '14:30', hour12: true });

    expect(field.hour.value).toBe('02');
    expect(field.minute.value).toBe('30');
    expect(field.period()?.value).toBe('pm');
  });

  it('shows midnight and noon as 12 on a 12-hour clock', () => {
    const midnight = renderTime({ value: '00:15', hour12: true });
    expect(midnight.hour.value).toBe('12');
    expect(midnight.period()?.value).toBe('am');
    expect(renderTime({ value: '12:15', hour12: true }).period()?.value).toBe('pm');
  });

  it('binds typed 24-hour entries as HH:mm and keeps partial digits', async () => {
    const field = renderTime({ hour12: false });

    await userEvent.type(field.hour, '1');
    expect(field.hour.value).toBe('1');
    await userEvent.type(field.hour, '3');
    expect(field.hour.value).toBe('13');
    expect(document.activeElement).toBe(field.minute);
    await userEvent.type(field.minute, '5');

    expect(field.bound()).toBe('13:05');
    await userEvent.tab();
    expect(field.minute.value).toBe('05');
  });

  it('converts 12-hour entries to a 24-hour value', async () => {
    const field = renderTime({ value: '09:00', hour12: true });

    await userEvent.selectOptions(field.period()!, 'pm');
    expect(field.bound()).toBe('21:00');

    await userEvent.clear(field.hour);
    await userEvent.type(field.hour, '12');
    expect(field.bound()).toBe('12:00');
  });

  it('stores 12 AM as hour 00', async () => {
    const field = renderTime({ value: '09:00', hour12: true });

    await userEvent.clear(field.hour);
    await userEvent.type(field.hour, '12');

    expect(field.bound()).toBe('00:00');
  });

  it('keeps the newest digits when the caret is at the end of a filled segment', async () => {
    const field = renderTime({ value: '10:30', hour12: false });
    field.minute.focus();
    field.minute.setSelectionRange(2, 2);

    await userEvent.keyboard('4');

    expect(field.minute.value).toBe('04');
    expect(field.bound()).toBe('10:04');
  });

  it.each([
    { clock: '24-hour', hour12: false, waits: '2', moves: '3' },
    { clock: '12-hour', hour12: true, waits: '1', moves: '2' }
  ])('moves to the minute only after a complete $clock hour', async ({ hour12, waits, moves }) => {
    const field = renderTime({ hour12 });

    await userEvent.type(field.hour, waits);
    expect(document.activeElement).toBe(field.hour);
    await userEvent.clear(field.hour);
    await userEvent.type(field.hour, moves);
    expect(document.activeElement).toBe(field.minute);
  });

  it.each([
    { name: 'hour 24 on a 24-hour clock', value: '24', segment: 'hour' as const, hour12: false },
    { name: 'hour 0 on a 12-hour clock', value: '00', segment: 'hour' as const, hour12: true },
    { name: 'minute 60', value: '60', segment: 'minute' as const, hour12: false }
  ])('binds an empty value for $name', async ({ value, segment, hour12 }) => {
    const field = renderTime({ value: '10:30', hour12 });

    await userEvent.clear(field[segment]);
    await userEvent.type(field[segment], value);

    expect(field.bound()).toBe('');
  });

  it('exposes segments as spin buttons with their ranges', () => {
    const field = renderTime({ value: '14:30', hour12: true });

    expect(field.hour.getAttribute('role')).toBe('spinbutton');
    expect(field.hour.getAttribute('aria-valuemin')).toBe('1');
    expect(field.hour.getAttribute('aria-valuemax')).toBe('12');
    expect(field.hour.getAttribute('aria-valuenow')).toBe('2');
    expect(field.minute.getAttribute('aria-valuemax')).toBe('59');
  });

  it('binds an empty value for an hour outside the clock', async () => {
    const field = renderTime({ value: '10:00', hour12: true });

    await userEvent.clear(field.hour);
    await userEvent.type(field.hour, '13');

    expect(field.hour.value).toBe('13');
    expect(field.bound()).toBe('');
  });

  it('selects a segment when it receives focus', async () => {
    const field = renderTime({ value: '10:30', hour12: false });

    await userEvent.click(field.minute);

    expect([field.minute.selectionStart, field.minute.selectionEnd]).toEqual([0, 2]);
  });

  it('accepts digits only', async () => {
    const field = renderTime({ hour12: false });

    await userEvent.type(field.hour, 'a1');

    expect(field.hour.value).toBe('1');
  });

  it('steps and wraps segments with the arrow keys', async () => {
    const field = renderTime({ value: '23:59', hour12: false });

    await userEvent.click(field.minute);
    await userEvent.keyboard('{ArrowUp}');
    expect(field.bound()).toBe('23:00');

    await userEvent.click(field.hour);
    await userEvent.keyboard('{ArrowUp}');
    expect(field.bound()).toBe('00:00');

    await userEvent.keyboard('{ArrowDown}');
    expect(field.bound()).toBe('23:00');
  });

  it('starts stepping an empty segment from zero', async () => {
    const field = renderTime({ hour12: false });

    await userEvent.click(field.minute);
    await userEvent.keyboard('{ArrowDown}');

    expect(field.minute.value).toBe('59');
  });

  it('wraps 12-hour clock hours between 12 and 1', async () => {
    const field = renderTime({ value: '12:00', hour12: true });

    await userEvent.click(field.hour);
    await userEvent.keyboard('{ArrowUp}');

    expect(field.hour.value).toBe('01');
    expect(field.bound()).toBe('13:00');
  });

  it('shows a value that the owner sets after mount', async () => {
    const field = renderTime({ hour12: false });
    await userEvent.type(field.hour, '1');

    field.setValue();

    await expect.poll(() => field.hour.value).toBe('07');
    expect(field.minute.value).toBe('05');
  });
});
