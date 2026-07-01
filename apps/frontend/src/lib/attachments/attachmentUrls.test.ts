import { describe, expect, it, vi } from 'vitest';
import { FitMode } from '$lib/render/types';
import type { AttachmentAPI } from '@chatto/api-client/attachments';
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

function apiWithRefresh(
  refreshMessageAttachmentUrls: AttachmentAPI['refreshMessageAttachmentUrls']
): Pick<AttachmentAPI, 'refreshMessageAttachmentUrls'> {
  return { refreshMessageAttachmentUrls };
}

describe('refreshAttachmentUrlsForMessage', () => {
  it('delegates refreshes to the attachment API with default thumbnail options', async () => {
    const urlsFromAPI = new Map<string, RefreshedAttachmentUrls>([
      [
        'att_1',
        {
          assetUrl: {
            url: 'https://cdn.example.com/fresh-1.jpg',
            expiresAt: '2026-05-29T15:00:00Z'
          },
          thumbnailAssetUrl: null,
          videoThumbnailAssetUrl: {
            url: 'https://cdn.example.com/video-thumb.jpg',
            expiresAt: '2026-05-29T15:00:00Z'
          },
          variantAssetUrls: new Map([
            [
              '720p',
              {
                url: 'https://cdn.example.com/video-720.mp4',
                expiresAt: '2026-05-29T15:00:00Z'
              }
            ]
          ])
        }
      ],
      [
        'att_2',
        {
          assetUrl: {
            url: 'https://cdn.example.com/fresh-2.jpg',
            expiresAt: '2026-05-29T15:00:00Z'
          },
          thumbnailAssetUrl: null,
          videoThumbnailAssetUrl: null,
          variantAssetUrls: new Map()
        }
      ]
    ]);
    const refreshMessageAttachmentUrls = vi.fn().mockResolvedValue(urlsFromAPI);

    const urls = await refreshAttachmentUrlsForMessage(
      apiWithRefresh(refreshMessageAttachmentUrls),
      'room_1',
      'event_1'
    );

    expect(refreshMessageAttachmentUrls).toHaveBeenCalledWith('room_1', 'event_1', {
      width: 960,
      height: 800,
      fit: FitMode.Contain
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

  it('passes caller-selected thumbnail shape to the attachment API', async () => {
    const refreshMessageAttachmentUrls = vi.fn().mockResolvedValue(new Map());

    await refreshAttachmentUrlsForMessage(
      apiWithRefresh(refreshMessageAttachmentUrls),
      'room_1',
      'event_1',
      {
        width: 120,
        height: 120,
        fit: FitMode.Cover
      }
    );

    expect(refreshMessageAttachmentUrls).toHaveBeenCalledWith('room_1', 'event_1', {
      width: 120,
      height: 120,
      fit: FitMode.Cover
    });
  });

  it('returns an empty map when the refresh request fails', async () => {
    const refreshMessageAttachmentUrls = vi.fn().mockRejectedValue(new Error('network failed'));

    const urls = await refreshAttachmentUrlsForMessage(
      apiWithRefresh(refreshMessageAttachmentUrls),
      'room_1',
      'event_1'
    );

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
