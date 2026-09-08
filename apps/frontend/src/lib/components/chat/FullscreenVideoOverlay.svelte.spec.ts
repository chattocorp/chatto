import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { fullscreenVideo } from '$lib/state/globals.svelte';
import FullscreenVideoOverlay from './FullscreenVideoOverlay.svelte';

afterEach(() => {
  fullscreenVideo.close();
});

describe('FullscreenVideoOverlay', () => {
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
});
