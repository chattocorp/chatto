import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { fullscreenVideo } from '$lib/state/globals.svelte';
import FullscreenVideoOverlay from './FullscreenVideoOverlay.svelte';

afterEach(() => {
  fullscreenVideo.close();
  vi.restoreAllMocks();
});

describe('FullscreenVideoOverlay', () => {
  it('requests native fullscreen when the lazily mounted overlay is ready', async () => {
    const request = vi.spyOn(HTMLElement.prototype, 'requestFullscreen').mockResolvedValue();
    fullscreenVideo.open('https://chat.example.test/clip.mp4', null, 0);
    const { container } = render(FullscreenVideoOverlay);
    await expect.poll(() => request.mock.calls.length).toBe(1);
    expect(request.mock.contexts[0]).toBe(container.querySelector('.fullscreen-overlay'));
  });

  it('keeps the close button inside iOS safe areas', async () => {
    fullscreenVideo.open('https://chat.example.test/clip.mp4', null, 0);
    const { container } = render(FullscreenVideoOverlay);

    await expect.poll(() => container.querySelector('button')).toBeTruthy();
    const closeButton = container.querySelector<HTMLButtonElement>('button')!;

    expect(closeButton.className).toContain('safe-area-inset-top');
    expect(closeButton.className).toContain('safe-area-inset-right');
    expect(closeButton.type).toBe('button');

    closeButton.click();
    expect(fullscreenVideo.isOpen).toBe(false);
  });

  it('replaces an HLS source with a freshly authorised URL after an error', async () => {
    fullscreenVideo.open(
      { src: 'https://chat.example.test/old.m3u8', type: 'application/vnd.apple.mpegurl' },
      null,
      0,
      null,
      async () => ({
        src: 'https://chat.example.test/new.m3u8?retry=1',
        type: 'application/vnd.apple.mpegurl'
      })
    );

    await expect(fullscreenVideo.recover()).resolves.toBe(true);
    expect(fullscreenVideo.source?.src).toBe('https://chat.example.test/new.m3u8?retry=1');
  });
});
