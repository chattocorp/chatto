import { afterEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import VideoProcessingAnimation from './VideoProcessingAnimation.svelte';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('pauses offscreen and hidden, redraws with reduced motion, and cancels on unmount', async () => {
  let intersect!: IntersectionObserverCallback;
  const motion = Object.assign(new EventTarget(), { matches: false });
  vi.spyOn(window, 'matchMedia').mockReturnValue(motion as MediaQueryList);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersect = callback;
      }
      observe() {}
      disconnect() {}
    }
  );
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      disconnect() {}
    }
  );
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const draw = vi.spyOn(CanvasRenderingContext2D.prototype, 'drawImage');
  const view = render(VideoProcessingAnimation, { label: 'Processing' });
  await tick();
  await vi.advanceTimersByTimeAsync(1);
  const first = draw.mock.calls.length;
  expect(first).toBeGreaterThan(0);
  const intersection = (visible: boolean) =>
    intersect(
      [{ isIntersecting: visible } as IntersectionObserverEntry],
      {} as IntersectionObserver
    );
  intersection(false);
  await vi.advanceTimersByTimeAsync(1000);
  expect(draw).toHaveBeenCalledTimes(first);
  document.dispatchEvent(new Event('visibilitychange'));
  await vi.advanceTimersByTimeAsync(1000);
  expect(draw).toHaveBeenCalledTimes(first);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  intersection(true);
  await vi.advanceTimersByTimeAsync(1000);
  expect(draw).toHaveBeenCalledTimes(first);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  document.dispatchEvent(new Event('visibilitychange'));
  await vi.advanceTimersByTimeAsync(1);
  expect(draw).toHaveBeenCalledTimes(first + 1);
  motion.matches = true;
  motion.dispatchEvent(new Event('change'));
  await vi.advanceTimersByTimeAsync(1000);
  expect(draw).toHaveBeenCalledTimes(first + 2);
  motion.matches = false;
  motion.dispatchEvent(new Event('change'));
  await view.unmount();
  await vi.advanceTimersByTimeAsync(1000);
  expect(draw).toHaveBeenCalledTimes(first + 2);
});
