import { describe, expect, it, vi } from 'vitest';
import type { Client } from '@urql/svelte';
import { FitMode } from '$lib/gql/graphql';
import {
  ASSET_URL_REFRESH_LEAD_MS,
  assetUrlExpiresAtMs,
  assetUrlNeedsRefresh,
  assetUrlRefreshAt,
  earliestAssetUrlRefreshAt,
  mergeRefreshedAttachmentUrls,
  refreshAttachmentUrlsForMessage,
  withAssetUrlRetryParam,
  type RefreshedAttachmentUrls
} from './attachmentUrls';

function clientWith(result: unknown): Client {
  return {
    query: vi.fn(() => ({
      toPromise: () => Promise.resolve(result)
    }))
  } as unknown as Client;
}

describe('refreshAttachmentUrlsForMessage', () => {
  it('extracts fresh URLs from a message event response', async () => {
    const query = vi.fn(() => ({
      toPromise: () =>
        Promise.resolve({
          data: {
            room: {
              event: {
                event: {
                  attachments: [
                    {
                      id: 'att_1',
                      assetUrl: {
                        url: 'https://cdn.example.com/fresh-1.jpg',
                        expiresAt: '2026-05-29T15:00:00Z'
                      },
                      thumbnailAssetUrl: null,
                      videoProcessing: {
                        thumbnailAssetUrl: {
                          url: 'https://cdn.example.com/video-thumb.jpg',
                          expiresAt: '2026-05-29T15:00:00Z'
                        },
                        variants: [
                          {
                            quality: '720p',
                            assetUrl: {
                              url: 'https://cdn.example.com/video-720.mp4',
                              expiresAt: '2026-05-29T15:00:00Z'
                            }
                          }
                        ]
                      }
                    },
                    {
                      id: 'att_2',
                      assetUrl: {
                        url: 'https://cdn.example.com/fresh-2.jpg',
                        expiresAt: '2026-05-29T15:00:00Z'
                      },
                      thumbnailAssetUrl: null,
                      videoProcessing: null
                    }
                  ]
                }
              }
            }
          }
        })
    }));
    const client = { query } as unknown as Client;

    const urls = await refreshAttachmentUrlsForMessage(client, 'room_1', 'event_1');

    expect(query).toHaveBeenCalledWith(expect.anything(), {
      roomId: 'room_1',
      eventId: 'event_1',
      thumbnailWidth: 960,
      thumbnailHeight: 800,
      thumbnailFit: FitMode.Contain
    });
    expect(urls.get('att_1')?.assetUrl.url).toBe('https://cdn.example.com/fresh-1.jpg');
    expect(urls.get('att_1')?.videoThumbnailAssetUrl?.url).toBe(
      'https://cdn.example.com/video-thumb.jpg'
    );
    expect(urls.get('att_1')?.variantAssetUrls.get('720p')?.url).toBe(
      'https://cdn.example.com/video-720.mp4'
    );
    expect(urls.get('att_2')?.assetUrl.url).toBe('https://cdn.example.com/fresh-2.jpg');
  });

  it('passes caller-selected thumbnail shape to the refresh query', async () => {
    const client = clientWith({
      data: {
        room: {
          event: {
            event: {
              attachments: [
                {
                  id: 'att_1',
                  assetUrl: {
                    url: 'https://cdn.example.com/fresh-1.jpg',
                    expiresAt: '2026-05-29T15:00:00Z'
                  },
                  thumbnailAssetUrl: null,
                  videoProcessing: null
                }
              ]
            }
          }
        }
      }
    });

    await refreshAttachmentUrlsForMessage(client, 'room_1', 'event_1', {
      width: 120,
      height: 120,
      fit: FitMode.Cover
    });

    expect(client.query).toHaveBeenCalledWith(expect.anything(), {
      roomId: 'room_1',
      eventId: 'event_1',
      thumbnailWidth: 120,
      thumbnailHeight: 120,
      thumbnailFit: FitMode.Cover
    });
  });

  it('returns an empty map when the refresh query fails', async () => {
    const client = clientWith({
      error: new Error('network failed')
    });

    const urls = await refreshAttachmentUrlsForMessage(client, 'room_1', 'event_1');

    expect(urls.size).toBe(0);
  });
});

describe('asset URL expiry helpers', () => {
  const now = Date.parse('2026-05-29T14:00:00Z');

  it('parses valid expiry timestamps', () => {
    expect(assetUrlExpiresAtMs({ url: '/asset', expiresAt: '2026-05-29T15:00:00Z' })).toBe(
      Date.parse('2026-05-29T15:00:00Z')
    );
  });

  it('schedules refresh before expiry', () => {
    expect(assetUrlRefreshAt({ url: '/asset', expiresAt: '2026-05-29T15:00:00Z' })).toBe(
      Date.parse('2026-05-29T15:00:00Z') - ASSET_URL_REFRESH_LEAD_MS
    );
  });

  it('treats expired and near-expiry URLs as needing refresh', () => {
    expect(
      assetUrlNeedsRefresh({ url: '/expired', expiresAt: '2026-05-29T13:59:59Z' }, now)
    ).toBe(true);
    expect(
      assetUrlNeedsRefresh(
        { url: '/near', expiresAt: new Date(now + ASSET_URL_REFRESH_LEAD_MS).toISOString() },
        now
      )
    ).toBe(true);
    expect(
      assetUrlNeedsRefresh({ url: '/fresh', expiresAt: '2026-05-29T15:00:00Z' }, now)
    ).toBe(false);
  });

  it('treats malformed expiry timestamps as immediately refreshable', () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    expect(assetUrlExpiresAtMs({ url: '/asset', expiresAt: 'not-a-date' })).toBe(now);
    expect(assetUrlNeedsRefresh({ url: '/asset', expiresAt: 'not-a-date' }, now)).toBe(true);
    vi.useRealTimers();
  });

  it('finds the earliest refresh time across optional URLs', () => {
    expect(
      earliestAssetUrlRefreshAt([
        null,
        { url: '/later', expiresAt: '2026-05-29T15:00:00Z' },
        { url: '/earlier', expiresAt: '2026-05-29T14:30:00Z' }
      ])
    ).toBe(Date.parse('2026-05-29T14:30:00Z') - ASSET_URL_REFRESH_LEAD_MS);
  });

  it('merges refreshed URL maps without dropping existing entries', () => {
    const existing = new Map<string, RefreshedAttachmentUrls>([
      [
        'att_1',
        {
          assetUrl: { url: '/old-1', expiresAt: '2026-05-29T15:00:00Z' },
          thumbnailAssetUrl: null,
          videoThumbnailAssetUrl: null,
          variantAssetUrls: new Map()
        }
      ]
    ]);
    const fresh = new Map<string, RefreshedAttachmentUrls>([
      [
        'att_2',
        {
          assetUrl: { url: '/fresh-2', expiresAt: '2026-05-29T16:00:00Z' },
          thumbnailAssetUrl: null,
          videoThumbnailAssetUrl: null,
          variantAssetUrls: new Map()
        }
      ]
    ]);

    const merged = mergeRefreshedAttachmentUrls(existing, fresh);

    expect(merged.get('att_1')?.assetUrl.url).toBe('/old-1');
    expect(merged.get('att_2')?.assetUrl.url).toBe('/fresh-2');
  });

  it('appends retry params while preserving query strings and hashes', () => {
    expect(withAssetUrlRetryParam('/assets/files/A?access=ticket#view', 123)).toBe(
      '/assets/files/A?access=ticket&retry=123#view'
    );
    expect(withAssetUrlRetryParam('/assets/files/A', 'again')).toBe(
      '/assets/files/A?retry=again'
    );
  });
});
