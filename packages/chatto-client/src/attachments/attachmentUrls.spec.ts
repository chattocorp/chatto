// @vitest-environment node
import { describe, expect, it, vi } from 'vitest';
import {
  ASSET_URL_REFRESH_LEAD_MS,
  DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH,
  assetUrlExpiresAtMs,
  assetUrlNeedsRefresh,
  assetUrlRefreshAt,
  createAssetUrlRetainer,
  earliestAssetUrlRefreshAt,
  mergeRefreshedAttachmentUrls,
  refreshAttachmentUrlsForAssets,
  withAssetUrlRetryParam,
  type RefreshedAttachmentUrls
} from './attachmentUrls.js';

const at = (iso: string) => new Date(iso).getTime();
const url = (path: string, expiresAt = '2026-09-29T12:00:00Z') => ({
  url: `https://cdn.example${path}`,
  expiresAt
});

describe('asset URL expiry', () => {
  it('reads the expiry and refreshes the lead time before it', () => {
    const asset = url('/a?sig=1');
    expect(assetUrlExpiresAtMs(asset)).toBe(at('2026-09-29T12:00:00Z'));
    expect(assetUrlRefreshAt(asset)).toBe(at('2026-09-29T12:00:00Z') - ASSET_URL_REFRESH_LEAD_MS);
    expect(assetUrlRefreshAt(asset, 0)).toBe(at('2026-09-29T12:00:00Z'));
    expect(assetUrlExpiresAtMs(null)).toBeNull();
    expect(assetUrlRefreshAt(undefined)).toBeNull();
  });

  it('treats an unreadable expiry as expired now', () => {
    const now = Date.now();
    expect(assetUrlExpiresAtMs({ url: 'x', expiresAt: 'not a date' })).toBeGreaterThanOrEqual(now);
    expect(assetUrlNeedsRefresh({ url: 'x', expiresAt: 'not a date' })).toBe(true);
  });

  it('needs a refresh from the refresh time on, and never without a URL', () => {
    const asset = url('/a');
    const refreshAt = assetUrlRefreshAt(asset)!;
    expect(assetUrlNeedsRefresh(asset, refreshAt - 1)).toBe(false);
    expect(assetUrlNeedsRefresh(asset, refreshAt)).toBe(true);
    expect(assetUrlNeedsRefresh(null, refreshAt)).toBe(false);
  });

  it('finds the earliest refresh and skips missing URLs', () => {
    expect(earliestAssetUrlRefreshAt([])).toBeNull();
    expect(earliestAssetUrlRefreshAt([null, undefined])).toBeNull();
    expect(
      earliestAssetUrlRefreshAt(
        [url('/late', '2026-09-29T13:00:00Z'), null, url('/early', '2026-09-29T12:30:00Z')],
        0
      )
    ).toBe(at('2026-09-29T12:30:00Z'));
  });
});

describe('createAssetUrlRetainer', () => {
  it('keeps a usable URL across a new signature of the same asset', () => {
    const now = at('2026-09-29T11:00:00Z');
    const retain = createAssetUrlRetainer(() => now);
    const first = url('/asset.png?sig=1#frag');
    expect(retain('k', first)).toBe(first);
    expect(retain('k', url('/asset.png?sig=2'))).toBe(first);
  });

  it('accepts a forced, expired, or different URL, and forgets a removed one', () => {
    let now = at('2026-09-29T11:00:00Z');
    const retain = createAssetUrlRetainer(() => now);
    const first = url('/asset.png?sig=1');
    retain('k', first);

    const forced = url('/asset.png?sig=2');
    expect(retain('k', forced, true)).toBe(forced);

    const other = url('/other.png?sig=1');
    expect(retain('k', other)).toBe(other);

    now = at('2026-09-29T12:00:00Z');
    const renewed = url('/other.png?sig=2', '2026-09-29T14:00:00Z');
    expect(retain('k', renewed)).toBe(renewed);

    expect(retain('k', null)).toBeNull();
    const again = url('/other.png?sig=3', '2026-09-29T14:00:00Z');
    expect(retain('k', again)).toBe(again);
  });
});

describe('attachment URL maps', () => {
  const entry = (path: string): RefreshedAttachmentUrls => ({
    assetUrl: url(path),
    thumbnailAssetUrl: null,
    videoThumbnailAssetUrl: null,
    variantAssetUrls: new Map()
  });

  it('merges fresh URLs over current ones and keeps the map without news', () => {
    const current = new Map([
      ['a', entry('/a1')],
      ['b', entry('/b1')]
    ]);
    expect(mergeRefreshedAttachmentUrls(current, new Map())).toBe(current);
    const merged = mergeRefreshedAttachmentUrls(current, new Map([['a', entry('/a2')]]));
    expect(merged.get('a')?.assetUrl?.url).toBe('https://cdn.example/a2');
    expect(merged.get('b')?.assetUrl?.url).toBe('https://cdn.example/b1');
    expect(current.get('a')?.assetUrl?.url).toBe('https://cdn.example/a1');
  });

  it('adds a retry parameter before the fragment, but not to local URLs', () => {
    expect(withAssetUrlRetryParam('https://cdn.example/a', 1)).toBe(
      'https://cdn.example/a?retry=1'
    );
    expect(withAssetUrlRetryParam('https://cdn.example/a?sig=x#t=5', 'two words')).toBe(
      'https://cdn.example/a?sig=x&retry=two%20words#t=5'
    );
    expect(withAssetUrlRetryParam('data:image/png;base64,AA', 1)).toBe('data:image/png;base64,AA');
    expect(withAssetUrlRetryParam('blob:https://app/1', 1)).toBe('blob:https://app/1');
  });

  it('refreshes URLs with the default thumbnail size and returns none on failure', async () => {
    const fresh = new Map([['a', entry('/a')]]);
    const api = { refreshAssetUrls: vi.fn(async () => fresh) };
    await expect(refreshAttachmentUrlsForAssets(api, 'R1', ['a'])).resolves.toBe(fresh);
    expect(api.refreshAssetUrls).toHaveBeenCalledWith(
      'R1',
      ['a'],
      DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH
    );

    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    api.refreshAssetUrls.mockRejectedValueOnce(new Error('offline'));
    await expect(refreshAttachmentUrlsForAssets(api, 'R1', ['a'])).resolves.toEqual(new Map());
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});
