import { afterEach, describe, expect, it, vi } from 'vitest';
import { isHLSProvider, type HLSProvider } from 'vidstack';
import type { MediaPlayerElement } from 'vidstack/elements';
import 'vidstack/player';

let player: MediaPlayerElement | undefined;

function destroyPlayer() {
  player?.destroy();
  player?.remove();
  player = undefined;
}

afterEach(destroyPlayer);

/** Hold the real library import until the test has disposed of its provider. */
async function pendingHLSPlayer() {
  const library = await import('hls.js');
  const pending = Promise.withResolvers<typeof library>();
  const load = vi.fn(() => pending.promise);
  let provider: HLSProvider | undefined;
  player = document.createElement('media-player');
  // Assert error events directly; Vidstack's debug logs contain circular player state.
  player.logLevel = 'silent';
  player.addEventListener('provider-change', (event) => {
    if (isHLSProvider(event.detail)) {
      provider = event.detail;
      provider.library = () => load();
    }
  });
  player.append(document.createElement('media-provider'));
  player.src = {
    src: 'data:application/vnd.apple.mpegurl,%23EXTM3U',
    type: 'application/vnd.apple.mpegurl'
  };
  document.body.append(player);
  player.startLoading();
  await expect.poll(() => load.mock.calls.length).toBe(1);
  expect(provider).toBeDefined();
  return { player, provider: provider!, pending, library };
}

// The import's promise chain must finish before checking for late events or setup.
async function flushLibraryCompletion() {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
}

describe('HLS provider library lifecycle', () => {
  it.each(['resolve', 'reject'] as const)(
    'ignores a library that %ss after the player is destroyed',
    async (completion) => {
      const { player, provider, pending, library } = await pendingHLSPlayer();
      const loaded = vi.fn();
      const failed = vi.fn();
      player.addEventListener('hls-lib-loaded', () => loaded());
      player.addEventListener('hls-lib-load-error', (event) => failed(event.detail.message));

      destroyPlayer();
      if (completion === 'resolve') pending.resolve(library);
      else pending.reject(new Error('delayed library failure'));
      await flushLibraryCompletion();

      expect(loaded).not.toHaveBeenCalled();
      expect(failed).not.toHaveBeenCalled();
      expect(provider.instance).toBeNull();
    }
  );

  it.each(['resolve', 'reject'] as const)(
    'ignores a library that %ss after switching to an MP4 provider',
    async (completion) => {
      const { player, provider, pending, library } = await pendingHLSPlayer();
      const loaded = vi.fn();
      const failed = vi.fn();
      player.addEventListener('hls-lib-loaded', () => loaded());
      player.addEventListener('hls-lib-load-error', (event) => failed(event.detail.message));

      player.src = { src: 'data:video/mp4;base64,', type: 'video/mp4' };
      await expect.poll(() => player.provider?.type).toBe('video');
      if (completion === 'resolve') pending.resolve(library);
      else pending.reject(new Error('replaced library failure'));
      await flushLibraryCompletion();

      expect(loaded).not.toHaveBeenCalled();
      expect(failed).not.toHaveBeenCalled();
      expect(provider.instance).toBeNull();
      expect(player.provider?.type).toBe('video');
    }
  );

  it('reports a library failure while the player is active', async () => {
    const { player, pending } = await pendingHLSPlayer();
    const failed = vi.fn();
    player.addEventListener('hls-lib-load-error', (event) => failed(event.detail.message));
    pending.reject(new Error('active library failure'));

    await expect.poll(() => failed.mock.calls.length).toBe(1);
    expect(failed.mock.calls[0][0]).toBe('active library failure');
  });
});
