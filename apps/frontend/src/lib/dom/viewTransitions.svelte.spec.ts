import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { stopViewTransition, transitionTheme } from './viewTransitions';

let motion: EventTarget & { matches: boolean };
let update: () => void | Promise<void>;
let completeUpdate: () => void;
let finish: () => void;
let transition: ViewTransition;
let start: ReturnType<typeof vi.spyOn>;
let button: HTMLButtonElement;

function chooseTheme(apply: () => void) {
  button.onclick = (event) => transitionTheme(event, apply);
  button.click();
}

beforeEach(() => {
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
  const updateCallbackDone = new Promise<void>((resolve) => (completeUpdate = resolve));
  const finished = new Promise<void>((resolve) => (finish = resolve));
  transition = {
    ready: Promise.resolve(),
    updateCallbackDone,
    finished,
    skipTransition: vi.fn(),
    types: new Set<string>()
  } as ViewTransition;
  start = vi.spyOn(document, 'startViewTransition').mockImplementation((callback) => {
    if (typeof callback !== 'function') throw new Error('Expected an update callback');
    update = callback;
    return transition;
  });
  button = document.createElement('button');
  document.body.append(button);
});

afterEach(async () => {
  completeUpdate();
  finish();
  await vi.waitFor(() => expect(document.documentElement.dataset.viewMotion).toBeUndefined());
  button.remove();
  vi.restoreAllMocks();
});

it('applies theme changes immediately with reduced motion or no API', () => {
  const apply = vi.fn();
  motion.matches = true;
  chooseTheme(apply);
  expect(apply).toHaveBeenCalledOnce();
  expect(start).not.toHaveBeenCalled();
  motion.matches = false;
  Object.defineProperty(document, 'startViewTransition', { configurable: true, value: undefined });
  chooseTheme(apply);
  expect(apply).toHaveBeenCalledTimes(2);
});

it('uses the control centre for keyboard activation and clears its CSS state', async () => {
  const apply = vi.fn();
  const bounds = button.getBoundingClientRect();
  chooseTheme(apply);
  expect(apply).not.toHaveBeenCalled();
  expect(document.documentElement.style.getPropertyValue('--theme-x')).toBe(
    `${bounds.left + bounds.width / 2}px`
  );
  await update();
  expect(apply).toHaveBeenCalledOnce();
  finish();
  await vi.waitFor(() =>
    expect(document.documentElement.style.getPropertyValue('--theme-x')).toBe('')
  );
});

it('keeps the last theme choice when clicks precede the snapshot update', async () => {
  const first = vi.fn();
  const second = vi.fn();
  const last = vi.fn();
  chooseTheme(first);
  chooseTheme(second);
  chooseTheme(last);
  await update();
  completeUpdate();
  await vi.waitFor(() => expect(last).toHaveBeenCalledOnce());
  expect(first).not.toHaveBeenCalled();
  expect(second).not.toHaveBeenCalled();
  expect(start).toHaveBeenCalledOnce();
  expect(transition.skipTransition).toHaveBeenCalled();
});

it('navigation never starts an animation and stops an active theme reveal', () => {
  expect(stopViewTransition()).toBeUndefined();
  expect(start).not.toHaveBeenCalled();
  chooseTheme(vi.fn());
  stopViewTransition();
  expect(start).toHaveBeenCalledOnce();
  expect(transition.skipTransition).toHaveBeenCalledOnce();
});

it('stops the theme reveal when reduced motion is enabled', () => {
  chooseTheme(vi.fn());
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  expect(transition.skipTransition).toHaveBeenCalledOnce();
});
