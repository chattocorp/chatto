import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  AssetFitMode,
  RefreshMessageAttachmentUrlsRequest,
  RefreshMessageAttachmentUrlsResponse
} from '$lib/pb/chatto/api/v1/chat_pb';
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

const { fetchMock } = vi.hoisted(() => ({
  fetchMock: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: (id: string) => id === 'origin',
    getServer: (id: string) => {
      if (id === 'origin') return { id, url: 'http://localhost', token: null };
      if (id === 'remote') return { id, url: 'https://remote.example.test/', token: 'remote-token' };
      return undefined;
    }
  }
}));

function expiring(url: string, iso = '2026-05-29T15:00:00Z') {
  return {
    url,
    expiresAt: Timestamp.fromDate(new Date(iso))
  };
}

function refreshResponse(): Response {
  const bytes = new RefreshMessageAttachmentUrlsResponse({
    attachments: [
      {
        id: 'att_1',
        assetUrl: expiring('https://cdn.example.com/fresh-1.jpg'),
        thumbnailAssetUrl: undefined,
        videoProcessing: {
          thumbnailAssetUrl: expiring('https://cdn.example.com/video-thumb.jpg'),
          variants: [
            {
              quality: '720p',
              assetUrl: expiring('https://cdn.example.com/video-720.mp4')
            }
          ]
        }
      },
      {
        id: 'att_2',
        assetUrl: expiring('https://cdn.example.com/fresh-2.jpg'),
        thumbnailAssetUrl: undefined,
        videoProcessing: undefined
      }
    ]
  }).toBinary();
  return new Response(
    bytes.slice().buffer as ArrayBuffer,
    {
      status: 200,
      headers: { 'Content-Type': 'application/protobuf' }
    }
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

describe('refreshAttachmentUrlsForMessage', () => {
  it('extracts fresh URLs from a protobuf response', async () => {
    fetchMock.mockResolvedValue(refreshResponse());

    const urls = await refreshAttachmentUrlsForMessage('room_1', 'event_1');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/rooms/room_1/attachments/urls/refresh');
    const headers = init.headers as Headers;
    expect(headers.get('Accept')).toBe('application/protobuf');
    expect(headers.get('Content-Type')).toBe('application/protobuf');
    const request = RefreshMessageAttachmentUrlsRequest.fromBinary(
      new Uint8Array(init.body as ArrayBuffer)
    );
    expect(request).toMatchObject({
      roomId: 'room_1',
      eventId: 'event_1',
      thumbnailWidth: 960,
      thumbnailHeight: 800,
      thumbnailFit: AssetFitMode.CONTAIN
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

  it('passes caller-selected thumbnail shape and server auth to the protobuf endpoint', async () => {
    fetchMock.mockResolvedValue(refreshResponse());

    await refreshAttachmentUrlsForMessage(
      'room 1',
      'event_1',
      {
        width: 120,
        height: 120,
        fit: AssetFitMode.COVER
      },
      'remote'
    );

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('https://remote.example.test/api/rooms/room%201/attachments/urls/refresh');
    const headers = init.headers as Headers;
    expect(headers.get('Authorization')).toBe('Bearer remote-token');
    const request = RefreshMessageAttachmentUrlsRequest.fromBinary(
      new Uint8Array(init.body as ArrayBuffer)
    );
    expect(request).toMatchObject({
      roomId: 'room 1',
      eventId: 'event_1',
      thumbnailWidth: 120,
      thumbnailHeight: 120,
      thumbnailFit: AssetFitMode.COVER
    });
  });

  it('returns an empty map when the refresh request fails', async () => {
    fetchMock.mockResolvedValue(new Response('network failed', { status: 503 }));

    const urls = await refreshAttachmentUrlsForMessage('room_1', 'event_1');

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
