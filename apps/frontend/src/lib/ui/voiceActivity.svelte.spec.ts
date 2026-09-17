import '../../app.css';
import { afterEach, expect, it, vi } from 'vitest';
import { voiceActivity } from './voiceActivity';

const cleanups: Array<() => void> = [];
afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
  vi.restoreAllMocks();
});

function mount(level: () => number) {
  const canvas = document.createElement('canvas');
  canvas.style.cssText = 'width:280px;height:48px;color:rgb(60,120,240)';
  document.body.append(canvas);
  const detach = voiceActivity(level)(canvas);
  const cleanup = () => {
    detach?.();
    canvas.remove();
  };
  cleanups.push(cleanup);
  return { canvas, cleanup };
}

function hasPixels(canvas: HTMLCanvasElement) {
  return canvas
    .getContext('2d')!
    .getImageData(0, 0, canvas.width, canvas.height)
    .data.some((value, index) => index % 4 === 3 && value > 0);
}

it('draws speech, settles to transparent silence, and releases its sampler on detach', async () => {
  let level = 0.6;
  const read = vi.fn(() => level);
  const { canvas, cleanup } = mount(read);
  await expect.poll(() => hasPixels(canvas)).toBe(true);
  level = 0;
  await expect.poll(() => canvas.dataset.active, { timeout: 4000 }).toBe('false');
  expect(hasPixels(canvas)).toBe(false);
  const raf = vi.spyOn(window, 'requestAnimationFrame');
  // Wait for another sample to prove that idle sampling does not schedule frames.
  const before = read.mock.calls.length;
  await expect.poll(() => read.mock.calls.length).toBeGreaterThan(before);
  expect(raf).not.toHaveBeenCalled();
  cleanup();
  const calls = read.mock.calls.length;
  // A second mounted layer keeps the shared clock alive after the first detaches.
  const next = vi.fn(() => 0);
  mount(next);
  await expect.poll(() => next.mock.calls.length).toBeGreaterThan(2);
  expect(read).toHaveBeenCalledTimes(calls);
});

it('increases the glow brightness and area with microphone volume', async () => {
  let level = 0.005;
  const { canvas } = mount(() => level);
  function brightness() {
    const pixels = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height).data;
    let total = 0;
    let area = 0;
    for (let i = 3; i < pixels.length; i += 4) {
      total += pixels[i];
      if (pixels[i] > 20) area++;
    }
    return { total, area };
  }
  await expect.poll(() => brightness().area).toBeGreaterThan(500);
  const quiet = brightness();
  level = 0.3;
  await expect.poll(() => brightness().total).toBeGreaterThan(quiet.total * 1.5);
  await expect.poll(() => brightness().area).toBeGreaterThan(quiet.area * 1.3);
});

it('makes quiet speech visible above the bottom edge', async () => {
  const { canvas } = mount(() => 0.005);
  await expect
    .poll(
      () => {
        const scale = canvas.height / 48;
        // Quiet speech must fill a visible area, not only produce a faint bottom line.
        const pixels = canvas
          .getContext('2d')!
          .getImageData(0, 24 * scale, canvas.width, 18 * scale).data;
        let visible = 0;
        for (let i = 3; i < pixels.length; i += 4) if (pixels[i] > 20) visible++;
        return visible / (pixels.length / 4);
      },
      { timeout: 3500 }
    )
    .toBeGreaterThan(0.1);
});

it('paints a stationary tint without animation frames with reduced motion', async () => {
  const match = window.matchMedia.bind(window);
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => {
    const media = match(query);
    if (query.includes('prefers-reduced-motion'))
      Object.defineProperty(media, 'matches', { value: true });
    return media;
  });
  const raf = vi.spyOn(window, 'requestAnimationFrame');
  const { canvas } = mount(() => 0.7);
  await expect.poll(() => hasPixels(canvas)).toBe(true);
  expect(raf).not.toHaveBeenCalled();
});

it('does not sample or animate offscreen cards', async () => {
  const read = vi.fn(() => 1);
  const { canvas } = mount(read);
  canvas.style.position = 'fixed';
  canvas.style.top = '-1000px';
  const visible = vi.fn(() => 0);
  mount(visible);
  await expect.poll(() => visible.mock.calls.length).toBeGreaterThan(2);
  expect(read).not.toHaveBeenCalled();
  expect(canvas.dataset.active).toBe('false');
});
