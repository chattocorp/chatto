import { describe, it, expect, vi, afterEach } from 'vitest';
import type { ComposerLinkPreview } from '@chatto/client/api/linkPreviews';
import { LinkPreviewState } from './linkPreviews.svelte';
import { serverRegistry } from '$lib/client';
import type { RegisteredServer } from '@chatto/client/server/registry';

type FetchLinkPreview = (url: string) => Promise<ComposerLinkPreview | null>;

function apiWithFetch(fetchLinkPreview: FetchLinkPreview) {
  return { fetchLinkPreview };
}

describe('LinkPreviewState', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('skips known server, room, and user pills before selecting a web preview', async () => {
    vi.useFakeTimers();
    const servers: RegisteredServer[] = ['local', 'remote'].map((id) => ({
      id,
      url: id === 'local' ? window.location.origin : 'https://remote.example.com',
      name: id,
      iconUrl: null,
      token: null,
      userId: null,
      userLogin: null,
      userDisplayName: null,
      userAvatarUrl: null,
      reauthRequiredAt: null,
      addedAt: 1
    }));
    vi.spyOn(serverRegistry, 'servers', 'get').mockReturnValue(servers);
    vi.spyOn(serverRegistry, 'originServer', 'get').mockReturnValue(servers[0]);
    vi.spyOn(serverRegistry, 'getServer').mockImplementation((id) =>
      servers.find((server) => server.id === id)
    );
    const fetchLinkPreview = vi.fn<FetchLinkPreview>().mockResolvedValue(null);
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    for (const server of servers) {
      for (const path of ['/', '/chat/-', '/chat/-/R123456789abcde', '/chat/-/user/alice']) {
        const url = `${server.url}${path}`;
        const cleanup = state.scheduleDetection(url, false);
        await vi.advanceTimersByTimeAsync(500);
        cleanup();
        expect(state.detectedURLs).toEqual([]);
        expect(state.buildToken()).toBeNull();
      }
    }
    expect(fetchLinkPreview).not.toHaveBeenCalled();

    const cleanup = state.scheduleDetection(
      `${servers[0].url}/chat/-/R123456789abcde https://example.com/article`,
      false
    );
    await vi.advanceTimersByTimeAsync(500);
    cleanup();
    expect(state.detectedURLs).toEqual(['https://example.com/article']);
    expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith('https://example.com/article');
  });

  it('keeps previews for unknown server URLs', async () => {
    vi.useFakeTimers();
    const fetchLinkPreview = vi.fn<FetchLinkPreview>().mockResolvedValue(null);
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));
    const url = 'https://unknown.example.com/chat/-/R123456789abcde';
    const cleanup = state.scheduleDetection(url, false);
    await vi.advanceTimersByTimeAsync(500);
    cleanup();
    expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(url);
  });

  it('does not fetch OpenGraph data for Chatto message links', async () => {
    vi.useFakeTimers();
    const fetchLinkPreview = vi.fn<FetchLinkPreview>();
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    const cleanup = state.scheduleDetection(
      'See http://localhost/chat/-/R123456789abcde/m/Eabc123DEF456gh',
      false
    );
    await vi.advanceTimersByTimeAsync(500);
    cleanup();

    expect(state.detectedURLs).toEqual([
      'http://localhost/chat/-/R123456789abcde/m/Eabc123DEF456gh'
    ]);
    expect(fetchLinkPreview).not.toHaveBeenCalled();
  });

  it('does not fetch previews for ignored markdown URL regions or non-http URLs', async () => {
    vi.useFakeTimers();
    const fetchLinkPreview = vi.fn<FetchLinkPreview>();
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    for (const message of [
      '`https://example.com`',
      '\\`https://example.com\\`',
      '```\nhttps://example.com\n```',
      '> https://example.com',
      '<https://example.com>',
      '<https://example.com',
      'mail user@example.com',
      'ftp://example.com/file'
    ]) {
      const cleanup = state.scheduleDetection(message, false);
      await vi.advanceTimersByTimeAsync(500);
      cleanup();

      expect(state.detectedURLs).toEqual([]);
    }

    expect(fetchLinkPreview).not.toHaveBeenCalled();
  });

  it('fetches non-message links and exposes the active preview token', async () => {
    vi.useFakeTimers();
    const url = 'https://example.com/story';
    const fetchLinkPreview = vi.fn<FetchLinkPreview>().mockResolvedValue({
      url,
      previewToken: 'cht_LPpreviewtoken',
      title: 'Preview title',
      description: 'Preview description',
      imageUrl: null,
      siteName: 'Preview site',
      embedType: null,
      embedId: null,
      imageAssetId: 'asset_preview'
    });
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    const cleanup = state.scheduleDetection(
      `<https://suppressed.example/story> Look ${url}`,
      false
    );
    await vi.advanceTimersByTimeAsync(500);
    await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledOnce());
    cleanup();

    expect(fetchLinkPreview).toHaveBeenCalledWith(url);
    expect(state.buildToken()).toBe('cht_LPpreviewtoken');
  });

  it('dismisses active URLs and clears preview state', async () => {
    const state = new LinkPreviewState(() => apiWithFetch(vi.fn<FetchLinkPreview>()));
    state.detectedURLs = ['https://example.com'];
    state.previews.set('https://example.com', null);
    state.fetchingURLs.add('https://example.com');

    state.dismissPreview('https://example.com');
    expect(state.detectedURLs).toEqual([]);
    expect(state.dismissedURLs.has('https://example.com')).toBe(true);

    state.clear();
    expect(state.previews.size).toBe(0);
    expect(state.fetchingURLs.size).toBe(0);
    expect(state.dismissedURLs.size).toBe(0);
  });
});
