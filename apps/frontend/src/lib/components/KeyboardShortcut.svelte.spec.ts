import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import KeyboardShortcut from './KeyboardShortcut.svelte';

function press(key: string, extra: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', {
    key,
    bubbles: true,
    cancelable: true,
    ...extra
  });
  document.body.dispatchEvent(event);
  return event;
}

it('updates its binding and removes it when unmounted', async () => {
  const ontrigger = vi.fn();
  const view = render(KeyboardShortcut, { key: 'k', command: true, ontrigger });
  press('k', { metaKey: true });
  expect(ontrigger).toHaveBeenCalledTimes(1);
  await view.rerender({ key: 'j', command: false, alt: true });
  expect(press('k', { metaKey: true }).defaultPrevented).toBe(false);
  press('j', { altKey: true });
  expect(ontrigger).toHaveBeenCalledTimes(2);
  await view.unmount();
  expect(press('j', { altKey: true }).defaultPrevented).toBe(false);
  expect(ontrigger).toHaveBeenCalledTimes(2);
});

it('matches exact modifiers and consumes repeats without retriggering', () => {
  const ontrigger = vi.fn();
  render(KeyboardShortcut, { key: 'k', command: true, ontrigger });
  for (const extra of [
    {},
    { metaKey: true, ctrlKey: true },
    { metaKey: true, shiftKey: true },
    { metaKey: true, altKey: true },
    { metaKey: true, isComposing: true }
  ])
    expect(press('k', extra).defaultPrevented).toBe(false);
  expect(ontrigger).not.toHaveBeenCalled();
  expect(press('K', { ctrlKey: true }).defaultPrevented).toBe(true);
  expect(press('K', { ctrlKey: true, repeat: true }).defaultPrevented).toBe(true);
  expect(ontrigger).toHaveBeenCalledTimes(1);
});

it('evaluates eligibility at keydown time without consuming disabled shortcuts', () => {
  let enabled = false;
  const ontrigger = vi.fn();
  render(KeyboardShortcut, { key: 'k', when: () => enabled, ontrigger });
  expect(press('k').defaultPrevented).toBe(false);
  enabled = true;
  expect(press('k').defaultPrevented).toBe(true);
  expect(ontrigger).toHaveBeenCalledTimes(1);
});
