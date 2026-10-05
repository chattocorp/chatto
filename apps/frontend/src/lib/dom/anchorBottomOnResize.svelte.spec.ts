import { afterEach, describe, expect, it, vi } from 'vitest';
import { anchorBottomOnResize } from './anchorBottomOnResize';

const cleanups: Array<() => void> = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

/** Mount a 400px scroller with 2000px of content. */
function mountScroller(options: Parameters<typeof anchorBottomOnResize>[0]) {
  const scroller = document.createElement('div');
  scroller.style.cssText = 'height: 400px; overflow-y: auto;';
  const content = document.createElement('div');
  content.style.height = '2000px';
  scroller.append(content);
  document.body.append(scroller);
  const detach = anchorBottomOnResize(options)(scroller);
  cleanups.push(() => {
    detach();
    scroller.remove();
  });
  return scroller;
}

function distanceFromBottom(scroller: HTMLElement) {
  return scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
}

/** Wait until the ResizeObserver has delivered the latest size. */
function nextFrames() {
  return new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  );
}

describe('anchorBottomOnResize', () => {
  it('keeps a scroller that follows its end at the bottom when it gets shorter', async () => {
    const scroller = mountScroller({ followsBottom: () => true });
    scroller.scrollTop = scroller.scrollHeight;
    await nextFrames();

    scroller.style.height = '150px';

    await vi.waitFor(() => expect(distanceFromBottom(scroller)).toBe(0));
  });

  it('keeps the bottom edge in place for a scrolled-up reader', async () => {
    const scroller = mountScroller({ followsBottom: () => false });
    scroller.scrollTop = 600;
    await nextFrames();
    const distance = distanceFromBottom(scroller);

    scroller.style.height = '150px';
    await vi.waitFor(() => expect(scroller.scrollTop).toBe(850));
    expect(distanceFromBottom(scroller)).toBe(distance);

    scroller.style.height = '400px';
    await vi.waitFor(() => expect(scroller.scrollTop).toBe(600));
  });

  it('keeps the distance when the browser clamps a scroller that gets taller', async () => {
    const scroller = mountScroller({ followsBottom: () => false });
    scroller.style.height = '150px';
    await nextFrames();
    scroller.scrollTop = scroller.scrollHeight - 150 - 100;
    await nextFrames();
    expect(distanceFromBottom(scroller)).toBe(100);

    scroller.style.height = '400px';

    await vi.waitFor(() => expect(scroller.clientHeight).toBe(400));
    await nextFrames();
    expect(distanceFromBottom(scroller)).toBe(100);
  });

  it('leaves the position alone while another operation owns it', async () => {
    const scroller = mountScroller({ followsBottom: () => false, isPaused: () => true });
    scroller.scrollTop = 600;
    await nextFrames();

    scroller.style.height = '150px';
    await nextFrames();

    expect(scroller.scrollTop).toBe(600);
  });
});
