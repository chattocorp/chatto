import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { observeCanvas } from './observeCanvas';

let intersect: IntersectionObserverCallback;
let resize: ResizeObserverCallback;
let motion: EventTarget & { matches: boolean };
const disconnectResize = vi.fn();
const disconnectIntersection = vi.fn();
const cleanups: Array<() => void> = [];

beforeEach(() => {
  vi.clearAllMocks();
  motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(callback: ResizeObserverCallback) {
        resize = callback;
      }
      observe() {}
      disconnect = disconnectResize;
    }
  );
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
      }
      observe() {}
      disconnect = disconnectIntersection;
    }
  );
});

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup() {
  const onResize = vi.fn();
  const onVisibilityChange = vi.fn();
  const onMotionChange = vi.fn();
  const lifecycle = observeCanvas(document.createElement('canvas'), {
    onResize,
    onVisibilityChange,
    onMotionChange
  });
  cleanups.push(lifecycle.destroy);
  return { lifecycle, onResize, onVisibilityChange, onMotionChange };
}

function setIntersection(visible: boolean) {
  intersect([{ isIntersecting: visible } as IntersectionObserverEntry], {} as IntersectionObserver);
}

it('combines tab and element visibility without resuming an offscreen canvas', () => {
  const { lifecycle, onVisibilityChange } = setup();
  expect(lifecycle.visible).toBe(true);
  expect(onVisibilityChange).not.toHaveBeenCalled();
  setIntersection(false);
  expect(lifecycle.visible).toBe(false);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  document.dispatchEvent(new Event('visibilitychange'));
  setIntersection(true);
  expect(lifecycle.visible).toBe(false);
  setIntersection(false);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  document.dispatchEvent(new Event('visibilitychange'));
  expect(lifecycle.visible).toBe(false);
  setIntersection(true);
  expect(lifecycle.visible).toBe(true);
  expect(onVisibilityChange).toHaveBeenCalledTimes(6);
});

it('reads initial reduced motion and reports preference and size changes', () => {
  motion.matches = true;
  const { lifecycle, onMotionChange, onResize } = setup();
  expect(lifecycle.reducedMotion).toBe(true);
  motion.matches = false;
  motion.dispatchEvent(new Event('change'));
  expect(lifecycle.reducedMotion).toBe(false);
  expect(onMotionChange).toHaveBeenCalledOnce();
  resize([], {} as ResizeObserver);
  expect(onResize).toHaveBeenCalledOnce();
});

it('disconnects and discards queued callbacks after cleanup', () => {
  const { lifecycle, onResize, onVisibilityChange, onMotionChange } = setup();
  lifecycle.destroy();
  expect(lifecycle.visible).toBe(false);
  expect(disconnectResize).toHaveBeenCalledOnce();
  expect(disconnectIntersection).toHaveBeenCalledOnce();
  resize([], {} as ResizeObserver);
  setIntersection(true);
  document.dispatchEvent(new Event('visibilitychange'));
  motion.dispatchEvent(new Event('change'));
  expect(onResize).not.toHaveBeenCalled();
  expect(onVisibilityChange).not.toHaveBeenCalled();
  expect(onMotionChange).not.toHaveBeenCalled();
});
