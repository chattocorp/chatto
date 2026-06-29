import { FitMode } from '$lib/render/types';
import type { AttachmentAPI } from '@chatto/api-client/attachments';

export type ExpiringAssetUrl = {
  url: string;
  expiresAt: string;
};

export type RefreshedAttachmentUrls = {
  assetUrl: ExpiringAssetUrl;
  thumbnailAssetUrl: ExpiringAssetUrl | null;
  videoThumbnailAssetUrl: ExpiringAssetUrl | null;
  variantAssetUrls: Map<string, ExpiringAssetUrl>;
};

export type AttachmentThumbnailRefreshOptions = {
  width: number;
  height: number;
  fit: FitMode;
};

export const DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH: AttachmentThumbnailRefreshOptions = {
  width: 960,
  height: 800,
  fit: FitMode.Cover
};

export const ASSET_URL_REFRESH_LEAD_MS = 2 * 60_000;

export function assetUrlExpiresAtMs(assetUrl: ExpiringAssetUrl | null | undefined): number | null {
  if (!assetUrl) return null;
  const expiresAt = new Date(assetUrl.expiresAt).getTime();
  return Number.isNaN(expiresAt) ? Date.now() : expiresAt;
}

export function assetUrlRefreshAt(
  assetUrl: ExpiringAssetUrl | null | undefined,
  leadMs = ASSET_URL_REFRESH_LEAD_MS
): number | null {
  const expiresAt = assetUrlExpiresAtMs(assetUrl);
  return expiresAt === null ? null : expiresAt - leadMs;
}

export function assetUrlNeedsRefresh(
  assetUrl: ExpiringAssetUrl | null | undefined,
  now = Date.now(),
  leadMs = ASSET_URL_REFRESH_LEAD_MS
): boolean {
  const refreshAt = assetUrlRefreshAt(assetUrl, leadMs);
  return refreshAt !== null && refreshAt <= now;
}

export function earliestAssetUrlRefreshAt(
  assetUrls: Iterable<ExpiringAssetUrl | null | undefined>,
  leadMs = ASSET_URL_REFRESH_LEAD_MS
): number | null {
  let nextRefreshAt: number | null = null;
  for (const assetUrl of assetUrls) {
    const refreshAt = assetUrlRefreshAt(assetUrl, leadMs);
    if (refreshAt === null) continue;
    nextRefreshAt = nextRefreshAt === null ? refreshAt : Math.min(nextRefreshAt, refreshAt);
  }
  return nextRefreshAt;
}

export function mergeRefreshedAttachmentUrls(
  current: Map<string, RefreshedAttachmentUrls>,
  fresh: Map<string, RefreshedAttachmentUrls>
): Map<string, RefreshedAttachmentUrls> {
  if (fresh.size === 0) return current;
  return new Map([...current, ...fresh]);
}

export function withAssetUrlRetryParam(url: string, retry: string | number): string {
  const hashStart = url.indexOf('#');
  const base = hashStart === -1 ? url : url.slice(0, hashStart);
  const hash = hashStart === -1 ? '' : url.slice(hashStart);
  const separator = base.includes('?') ? '&' : '?';
  return `${base}${separator}retry=${encodeURIComponent(String(retry))}${hash}`;
}

export async function refreshAttachmentUrlsForMessage(
  api: Pick<AttachmentAPI, 'refreshMessageAttachmentUrls'>,
  roomId: string,
  eventId: string,
  thumbnailOptions = DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH
): Promise<Map<string, RefreshedAttachmentUrls>> {
  try {
    return await api.refreshMessageAttachmentUrls(roomId, eventId, thumbnailOptions);
  } catch (error) {
    console.warn('Failed to refresh attachment URLs', error);
    return new Map();
  }
}
