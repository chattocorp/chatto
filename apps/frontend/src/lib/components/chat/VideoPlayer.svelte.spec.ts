import { tick } from 'svelte';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { VideoProcessingStatus } from '$lib/render/types';
import VideoPlayer from './VideoPlayer.svelte';

const TRANSPARENT_THUMBNAIL = 'data:image/gif;base64,R0lGODlhAQABAAAAACwAAAAAAQABAAA=';

function renderAutoLoopVideo({ width, height }: { width: number; height: number }) {
  return render(VideoPlayer, {
    props: {
      status: VideoProcessingStatus.Completed,
      filename: 'clip.mp4',
      autoLoop: true,
      variants: [
        {
          url: 'https://chat.example.test/clip.mp4',
          quality: `${height}p`,
          width,
          height,
          size: 1024
        }
      ]
    }
  });
}

function renderPostedVideo({
  width,
  height,
  thumbnailUrl = null
}: {
  width: number;
  height: number;
  thumbnailUrl?: string | null;
}) {
  return render(VideoPlayer, {
    props: {
      status: VideoProcessingStatus.Completed,
      filename: 'clip.mp4',
      width,
      height,
      thumbnailUrl,
      variants: [
        {
          url: 'https://chat.example.test/clip.mp4',
          quality: `${height}p`,
          width,
          height,
          size: 1024
        }
      ]
    }
  });
}

function frame(container: HTMLElement): HTMLElement {
  const element = container.querySelector<HTMLElement>('.embed-frame');
  expect(element).not.toBeNull();
  return element!;
}

function video(container: HTMLElement): HTMLVideoElement {
  const element = container.querySelector<HTMLVideoElement>('video[data-autoloop]');
  expect(element).not.toBeNull();
  return element!;
}

async function playerVideo(container: HTMLElement): Promise<HTMLVideoElement> {
  await expect.poll(() => container.querySelector('media-provider video')).toBeTruthy();
  return container.querySelector<HTMLVideoElement>('media-provider video')!;
}

async function mediaPlayer(container: HTMLElement): Promise<HTMLElement> {
  await expect.poll(() => container.querySelector('media-player')).toBeTruthy();
  return container.querySelector<HTMLElement>('media-player')!;
}

async function posterImage(container: HTMLElement): Promise<HTMLImageElement> {
  await expect.poll(() => container.querySelector('.vds-poster img')).toBeTruthy();
  return container.querySelector<HTMLImageElement>('.vds-poster img')!;
}

describe('VideoPlayer', () => {
  it('frames 16:9 videos as 16:9 embeds', () => {
    const { container } = renderAutoLoopVideo({ width: 1600, height: 900 });

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 480 / 270');
    expect(video(container).className).toContain('h-full');
    expect(video(container).className).toContain('w-full');
  });

  it('preserves 4:3 autoloop videos instead of forcing 16:9', () => {
    const { container } = renderAutoLoopVideo({ width: 640, height: 480 });

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 427 / 320');
  });

  it('preserves near-square posted landscape videos', async () => {
    const { container } = renderPostedVideo({
      width: 1024,
      height: 768,
      thumbnailUrl: TRANSPARENT_THUMBNAIL
    });

    await mediaPlayer(container);
    const poster = await posterImage(container);

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 427 / 320');
    expect(getComputedStyle(poster).objectFit).toBe('contain');
  });

  it('preserves near-square posted portrait videos', async () => {
    const { container } = renderPostedVideo({
      width: 800,
      height: 1000,
      thumbnailUrl: TRANSPARENT_THUMBNAIL
    });

    const player = await mediaPlayer(container);
    const media = await playerVideo(container);
    const poster = await posterImage(container);

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 256 / 320');
    expect(
      Math.abs(media.getBoundingClientRect().width - player.getBoundingClientRect().width)
    ).toBeLessThanOrEqual(2);
    expect(
      Math.abs(media.getBoundingClientRect().height - player.getBoundingClientRect().height)
    ).toBeLessThanOrEqual(2);
    expect(getComputedStyle(player).aspectRatio).toBe('256 / 320');
    expect(getComputedStyle(media).objectFit).toBe('contain');
    expect(getComputedStyle(poster).objectFit).toBe('contain');
  });

  it('preserves true portrait posted videos', async () => {
    const { container } = renderPostedVideo({
      width: 1080,
      height: 1920,
      thumbnailUrl: TRANSPARENT_THUMBNAIL
    });

    await mediaPlayer(container);
    const poster = await posterImage(container);

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 180 / 320');
    expect(getComputedStyle(poster).objectFit).toBe('contain');
  });

  it('preserves exact 2:3 and 3:2 boundary videos', async () => {
    const portrait = renderPostedVideo({ width: 1000, height: 1500 });
    const landscape = renderPostedVideo({ width: 1500, height: 1000 });

    await mediaPlayer(portrait.container);
    await mediaPlayer(landscape.container);

    expect(frame(portrait.container).getAttribute('style')).toContain('aspect-ratio: 213 / 320');
    expect(frame(landscape.container).getAttribute('style')).toContain('aspect-ratio: 480 / 320');
  });

  it('corrects stale metadata after the browser loads intrinsic video dimensions', async () => {
    const { container } = renderAutoLoopVideo({ width: 1024, height: 768 });
    const media = video(container);

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 427 / 320');

    Object.defineProperty(media, 'videoWidth', { configurable: true, value: 1920 });
    Object.defineProperty(media, 'videoHeight', { configurable: true, value: 1080 });
    media.dispatchEvent(new Event('loadedmetadata'));
    await tick();

    expect(frame(container).getAttribute('style')).toContain('aspect-ratio: 480 / 270');
  });
});
