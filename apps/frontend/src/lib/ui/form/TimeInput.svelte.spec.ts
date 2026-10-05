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
    expect(renderTime({ value: '00:15', hour12: true }).hour.value).toBe('12');
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

  it('binds an empty value for an hour outside the clock', async () => {
    const field = renderTime({ value: '10:00', hour12: true });

    await userEvent.clear(field.hour);
    await userEvent.type(field.hour, '13');

    expect(field.hour.value).toBe('13');
    expect(field.bound()).toBe('');
  });

  it('replaces a filled segment when the user types into it', async () => {
    const field = renderTime({ value: '10:30', hour12: false });

    await userEvent.click(field.minute);
    await userEvent.keyboard('45');

    expect(field.bound()).toBe('10:45');
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
