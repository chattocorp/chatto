import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { startLoadingGradients } from './loadingGradients';

let shell: HTMLDivElement;
let motion: EventTarget & { matches: boolean };
let frames: Map<number, FrameRequestCallback>;
let stop: (() => void) | undefined;

function advance(time: number) {
  const callbacks = [...frames.values()];
  frames.clear();
  for (const callback of callbacks) callback(time);
}

beforeEach(() => {
  shell = document.createElement('div');
  document.body.append(shell);
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  frames = new Map();
  let id = 0;
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frames.set(++id, callback);
    return id;
  });
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    frames.delete(id);
  });
});

afterEach(() => {
  stop?.();
  stop = undefined;
  shell.remove();
  vi.restoreAllMocks();
});

it('uses a fresh seed for each shell', () => {
  const random = vi.spyOn(Math, 'random').mockReturnValue(0.1);
  stop = startLoadingGradients(shell);
  const first = shell.style.cssText;
  stop();
  random.mockReturnValue(0.8);
  stop = startLoadingGradients(shell);
  expect(shell.style.cssText).not.toBe(first);
  expect(shell.hasAttribute('data-noise')).toBe(true);
});

it('moves smoothly within bounded positions and sizes', () => {
  stop = startLoadingGradients(shell);
  const initial = shell.style.cssText;
  let previous: number[] | undefined;
  for (let time = 0; time <= 10000; time += 16) {
    advance(time);
    const values: number[] = [];
    for (const [name, centres] of [
      ['first', [35, 40, 50]],
      ['second', [65, 60, 50]]
    ] as const) {
      for (const [index, property] of ['x', 'y', 'size'].entries()) {
        const value = parseFloat(shell.style.getPropertyValue(`--loading-${name}-${property}`));
        const range = property === 'size' ? 10 : 20;
        expect(value).toBeGreaterThanOrEqual(centres[index] - range);
        expect(value).toBeLessThanOrEqual(centres[index] + range);
        values.push(value);
      }
    }
    if (previous) {
      values.forEach((value, index) => expect(Math.abs(value - previous![index])).toBeLessThan(1));
    }
    previous = values;
  }
  expect(shell.style.cssText).not.toBe(initial);
});

it('pauses for reduced motion at startup and when the preference changes', () => {
  motion.matches = true;
  stop = startLoadingGradients(shell);
  const initial = shell.style.cssText;
  expect(frames.size).toBe(0);
  motion.matches = false;
  motion.dispatchEvent(new Event('change'));
  advance(0);
  advance(1000);
  expect(shell.style.cssText).not.toBe(initial);
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  expect(frames.size).toBe(0);
});

it('excludes time spent in a hidden tab', () => {
  stop = startLoadingGradients(shell);
  advance(0);
  advance(1000);
  const before = shell.style.cssText;
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  document.dispatchEvent(new Event('visibilitychange'));
  expect(frames.size).toBe(0);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  document.dispatchEvent(new Event('visibilitychange'));
  advance(60000);
  expect(shell.style.cssText).toBe(before);
  advance(61000);
  expect(shell.style.cssText).not.toBe(before);
});

it.each([false, true])('cleans up on removal, including while paused (%s)', async (reduced) => {
  motion.matches = reduced;
  const removeMotion = vi.spyOn(motion, 'removeEventListener');
  const removeVisibility = vi.spyOn(document, 'removeEventListener');
  stop = startLoadingGradients(shell);
  shell.remove();
  await vi.waitFor(() => {
    expect(removeMotion).toHaveBeenCalledWith('change', expect.any(Function));
    expect(removeVisibility).toHaveBeenCalledWith('visibilitychange', expect.any(Function));
  });
  expect(frames.size).toBe(0);
  motion.matches = false;
  motion.dispatchEvent(new Event('change'));
  document.dispatchEvent(new Event('visibilitychange'));
  expect(frames.size).toBe(0);
});
