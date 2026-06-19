import { csrfFetch } from '$lib/auth/csrf';
import {
	AssetFitMode,
	RefreshMessageAttachmentUrlsRequest,
	RefreshMessageAttachmentUrlsResponse,
	type AssetUrl as ProtoAssetUrl,
	type AttachmentView
} from '$lib/pb/chatto/api/v1/chat_pb';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

const PROTOBUF_CONTENT_TYPE = 'application/protobuf';

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
  fit: AssetFitMode;
};

export const AttachmentThumbnailFit = AssetFitMode;

export const DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH: AttachmentThumbnailRefreshOptions = {
  width: 960,
  height: 800,
  fit: AssetFitMode.CONTAIN
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
  roomId: string,
  eventId: string,
  thumbnailOptions = DEFAULT_ATTACHMENT_THUMBNAIL_REFRESH,
  serverId = getActiveServer()
): Promise<Map<string, RefreshedAttachmentUrls>> {
  const fresh = new Map<string, RefreshedAttachmentUrls>();
  if (!serverId) return fresh;

  const requestBody = new RefreshMessageAttachmentUrlsRequest({
    roomId,
    eventId,
    thumbnailWidth: thumbnailOptions.width,
    thumbnailHeight: thumbnailOptions.height,
    thumbnailFit: thumbnailOptions.fit
  }).toBinary();

  let response: Response;
  try {
    response = await csrfFetch(refreshAttachmentUrlsUrl(serverId, roomId), {
      method: 'POST',
      headers: refreshAttachmentUrlsHeaders(serverId),
      body: protobufBody(requestBody)
    });
  } catch (error) {
    console.warn('Failed to refresh attachment URLs', error);
    return fresh;
  }

  if (!response.ok) {
    console.warn('Failed to refresh attachment URLs', await refreshErrorMessage(response));
    return fresh;
  }

  const decoded = RefreshMessageAttachmentUrlsResponse.fromBinary(
    new Uint8Array(await response.arrayBuffer())
  );
  for (const attachment of decoded.attachments) {
    const value = attachmentViewToRefreshedUrls(attachment);
    if (value) fresh.set(attachment.id, value);
  }
  return fresh;
}

function protobufBody(bytes: Uint8Array): ArrayBuffer {
  return bytes.slice().buffer as ArrayBuffer;
}

function attachmentViewToRefreshedUrls(
  attachment: AttachmentView
): RefreshedAttachmentUrls | null {
  const assetUrl = assetUrlToExpiring(attachment.assetUrl);
  if (!assetUrl) return null;

  const variantAssetUrls = new Map<string, ExpiringAssetUrl>();
  for (const variant of attachment.videoProcessing?.variants ?? []) {
    const variantAssetUrl = assetUrlToExpiring(variant.assetUrl);
    if (variant.quality && variantAssetUrl) {
      variantAssetUrls.set(variant.quality, variantAssetUrl);
    }
  }

  return {
    assetUrl,
    thumbnailAssetUrl: assetUrlToExpiring(attachment.thumbnailAssetUrl),
    videoThumbnailAssetUrl: assetUrlToExpiring(attachment.videoProcessing?.thumbnailAssetUrl),
    variantAssetUrls
  };
}

function assetUrlToExpiring(assetUrl: ProtoAssetUrl | undefined): ExpiringAssetUrl | null {
  if (!assetUrl?.url || !assetUrl.expiresAt) return null;
  return {
    url: assetUrl.url,
    expiresAt: assetUrl.expiresAt.toDate().toISOString()
  };
}

function refreshAttachmentUrlsUrl(serverId: string, roomId: string): string {
  const encodedRoomId = encodeURIComponent(roomId);
  if (serverRegistry.isOriginServer(serverId)) {
    return `/api/rooms/${encodedRoomId}/attachments/urls/refresh`;
  }
  const server = serverRegistry.getServer(serverId);
  if (!server) throw new Error(`Server "${serverId}" not found`);
  return `${server.url.replace(/\/$/, '')}/api/rooms/${encodedRoomId}/attachments/urls/refresh`;
}

function refreshAttachmentUrlsHeaders(serverId: string): Headers {
  const headers = new Headers({
    Accept: PROTOBUF_CONTENT_TYPE,
    'Content-Type': PROTOBUF_CONTENT_TYPE
  });
  const token = serverRegistry.getServer(serverId)?.token;
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  return headers;
}

async function refreshErrorMessage(response: Response): Promise<string> {
  const fallback = `Attachment URL refresh failed (${response.status})`;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    try {
      const body = (await response.json()) as { error?: unknown };
      if (typeof body.error === 'string' && body.error) return body.error;
    } catch {
      return fallback;
    }
  }
  const text = await response.text();
  return text || fallback;
}
