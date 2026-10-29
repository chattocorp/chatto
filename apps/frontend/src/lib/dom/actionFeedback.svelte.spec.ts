import { afterEach, expect, it, vi } from 'vitest';
import { CopyFeedback } from './actionFeedback';

let button: HTMLButtonElement | undefined;
const feedback = new CopyFeedback();
afterEach(() => {
  feedback.dispose();
  button?.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function control() {
  button = document.createElement('button');
  button.setAttribute('aria-label', 'Copy');
  button.title = 'Copy';
  document.body.append(button);
  return button;
}

it('restores copy labels and restarts the confirmation on repeated success', () => {
  vi.useFakeTimers();
  const button = control();
  feedback.show(button, 'Copied');
  expect(button.dataset.copied).toBe('true');
  expect(button.getAttribute('aria-label')).toBe('Copied');
  vi.advanceTimersByTime(1000);
  feedback.show(button, 'Copied');
  vi.advanceTimersByTime(1000);
  expect(button.dataset.copied).toBe('true');
  vi.advanceTimersByTime(400);
  expect(button.dataset.copied).toBeUndefined();
  expect(button.title).toBe('Copy');
  expect(button.getAttribute('aria-label')).toBe('Copy');
});

it('releases copy timers and labels when the owner unmounts', () => {
  vi.useFakeTimers();
  const button = control();
  feedback.show(button, 'Copied');
  feedback.dispose();
  expect(vi.getTimerCount()).toBe(0);
  expect(button.dataset.copied).toBeUndefined();
});
