import { describe, it, expect, vi, afterEach } from 'vitest';
import type { Client } from '@urql/svelte';
import { LinkPreviewState } from './linkPreviews.svelte';

function clientWithQuery(query: ReturnType<typeof vi.fn>): Client {
  return { query } as unknown as Client;
}

describe('LinkPreviewState', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not fetch OpenGraph data for Chatto message links', async () => {
    vi.useFakeTimers();
    const query = vi.fn();
    const state = new LinkPreviewState(() => clientWithQuery(query));

    const cleanup = state.scheduleDetection('See http://localhost/chat/-/room_456/m/evt_123', false);
    await vi.advanceTimersByTimeAsync(500);
    cleanup();

    expect(state.detectedURLs).toEqual(['http://localhost/chat/-/room_456/m/evt_123']);
    expect(query).not.toHaveBeenCalled();
  });

  it('does not fetch previews for ignored markdown URL regions or non-http URLs', async () => {
    vi.useFakeTimers();
    const query = vi.fn();
    const state = new LinkPreviewState(() => clientWithQuery(query));

    for (const message of [
      '`https://example.com`',
      '\\`https://example.com\\`',
      '```\nhttps://example.com\n```',
      '> https://example.com',
      'mail user@example.com',
      'ftp://example.com/file'
    ]) {
      const cleanup = state.scheduleDetection(message, false);
      await vi.advanceTimersByTimeAsync(500);
      cleanup();

      expect(state.detectedURLs).toEqual([]);
    }

    expect(query).not.toHaveBeenCalled();
  });

  it('fetches non-message links and converts the active preview into mutation input', async () => {
    vi.useFakeTimers();
    const url = 'https://example.com/story';
    const query = vi.fn().mockResolvedValue({
      data: {
        linkPreview: {
          url,
          title: 'Preview title',
          description: 'Preview description',
          imageUrl: null,
          siteName: 'Preview site',
          embedType: null,
          embedId: null,
          imageAssetId: 'asset_preview'
        }
      },
      error: null
    });
    const state = new LinkPreviewState(() => clientWithQuery(query));

    const cleanup = state.scheduleDetection(`Look ${url}`, false);
    await vi.advanceTimersByTimeAsync(500);
    await vi.waitFor(() => expect(query).toHaveBeenCalledOnce());
    cleanup();

    expect(state.buildInput()).toMatchObject({
      url,
      title: 'Preview title',
      description: 'Preview description',
      siteName: 'Preview site',
      imageAssetId: 'asset_preview'
    });
  });

  it('dismisses active URLs and clears preview state', async () => {
    const state = new LinkPreviewState(() => clientWithQuery(vi.fn()));
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
