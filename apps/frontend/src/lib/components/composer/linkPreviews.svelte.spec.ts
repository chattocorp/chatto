import { describe, it, expect, vi, afterEach } from 'vitest';
import type { ComposerLinkResult } from '$lib/api-client/linkPreviews';
import { LinkPreviewState } from './linkPreviews.svelte';

type FetchLinkPreview = (url: string, roomId?: string) => Promise<ComposerLinkResult | null>;

function apiWithFetch(fetchLinkPreview: FetchLinkPreview) {
  return { fetchLinkPreview };
}

describe('LinkPreviewState', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not fetch OpenGraph data for Chatto message links', async () => {
    vi.useFakeTimers();
    const fetchLinkPreview = vi.fn<FetchLinkPreview>();
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    const cleanup = state.scheduleDetection(
      'See http://localhost/chat/-/room_456/m/evt_123',
      false
    );
    await vi.advanceTimersByTimeAsync(500);
    cleanup();

    expect(state.detectedURLs).toEqual(['http://localhost/chat/-/room_456/m/evt_123']);
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

  it('fetches non-message links and converts the active preview into mutation input', async () => {
    vi.useFakeTimers();
    const url = 'https://example.com/story';
    const fetchLinkPreview = vi.fn<FetchLinkPreview>().mockResolvedValue({
      kind: 'preview',
      preview: {
        url,
        previewToken: 'cht_LPpreviewtoken',
        title: 'Preview title',
        description: 'Preview description',
        imageUrl: null,
        siteName: 'Preview site',
        embedType: null,
        embedId: null,
        imageAssetId: 'asset_preview'
      }
    });
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    const cleanup = state.scheduleDetection(`Look ${url}`, false);
    await vi.advanceTimersByTimeAsync(500);
    await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledOnce());
    cleanup();

    expect(state.buildInput()).toMatchObject({
      previewToken: 'cht_LPpreviewtoken'
    });
  });

  it('turns a direct image result into an attachment asset ID', async () => {
    vi.useFakeTimers();
    const url = 'https://example.com/image.gif';
    const fetchLinkPreview = vi.fn<FetchLinkPreview>().mockResolvedValue({
      kind: 'attachment',
      attachment: {
        assetId: 'asset_linked',
        filename: 'linked-image.gif',
        contentType: 'image/gif',
        size: 1024n,
        width: 320,
        height: 180,
        previewUrl: '/assets/files/asset_linked/image'
      }
    });
    const state = new LinkPreviewState(() => apiWithFetch(fetchLinkPreview));

    state.scheduleDetection(url, false, 'room_1');
    await vi.advanceTimersByTimeAsync(500);
    await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledOnce());

    expect(fetchLinkPreview).toHaveBeenCalledWith(url, 'room_1');
    expect(state.buildInput()).toBeNull();
    expect(state.buildAttachmentAssetIds()).toEqual(['asset_linked']);
    expect(state.activeImportedAttachment?.contentType).toBe('image/gif');
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
