import { csrfFetch } from '$lib/auth/csrf';
import {
  ServerBrandingAssetResponse,
  UserAvatarAssetResponse
} from '$lib/pb/chatto/api/v1/chat_pb';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

const PROTOBUF_CONTENT_TYPE = 'application/protobuf';

export async function uploadUserAvatar(
  userId: string,
  file: File,
  serverId = getActiveServer()
): Promise<UserAvatarAssetResponse> {
  const resolvedServerId = requireServerId(serverId);
  const form = new FormData();
  form.set('avatar', file, file.name);

  const response = await csrfFetch(
    profileAssetUrl(resolvedServerId, `/api/users/${encodeURIComponent(userId)}/avatar`),
    {
      method: 'POST',
      headers: profileAssetHeaders(resolvedServerId),
      body: form
    }
  );
  if (!response.ok) {
    throw new Error(await profileAssetErrorMessage(response, 'Avatar upload failed'));
  }

  return UserAvatarAssetResponse.fromBinary(new Uint8Array(await response.arrayBuffer()));
}

export async function deleteUserAvatar(
  userId: string,
  serverId = getActiveServer()
): Promise<UserAvatarAssetResponse> {
  const resolvedServerId = requireServerId(serverId);
  const response = await csrfFetch(
    profileAssetUrl(resolvedServerId, `/api/users/${encodeURIComponent(userId)}/avatar`),
    {
      method: 'DELETE',
      headers: profileAssetHeaders(resolvedServerId)
    }
  );
  if (!response.ok) {
    throw new Error(await profileAssetErrorMessage(response, 'Avatar delete failed'));
  }

  return UserAvatarAssetResponse.fromBinary(new Uint8Array(await response.arrayBuffer()));
}

export function uploadServerLogo(
  file: File,
  serverId = getActiveServer()
): Promise<ServerBrandingAssetResponse> {
  return uploadServerBrandingAsset('logo', file, requireServerId(serverId));
}

export function deleteServerLogo(
  serverId = getActiveServer()
): Promise<ServerBrandingAssetResponse> {
  return deleteServerBrandingAsset('logo', requireServerId(serverId));
}

export function uploadServerBanner(
  file: File,
  serverId = getActiveServer()
): Promise<ServerBrandingAssetResponse> {
  return uploadServerBrandingAsset('banner', file, requireServerId(serverId));
}

export function deleteServerBanner(
  serverId = getActiveServer()
): Promise<ServerBrandingAssetResponse> {
  return deleteServerBrandingAsset('banner', requireServerId(serverId));
}

async function uploadServerBrandingAsset(
  kind: 'logo' | 'banner',
  file: File,
  serverId: string
): Promise<ServerBrandingAssetResponse> {
  const form = new FormData();
  form.set(kind, file, file.name);

  const response = await csrfFetch(profileAssetUrl(serverId, `/api/server/${kind}`), {
    method: 'POST',
    headers: profileAssetHeaders(serverId),
    body: form
  });
  if (!response.ok) {
    throw new Error(await profileAssetErrorMessage(response, `Server ${kind} upload failed`));
  }

  return ServerBrandingAssetResponse.fromBinary(new Uint8Array(await response.arrayBuffer()));
}

async function deleteServerBrandingAsset(
  kind: 'logo' | 'banner',
  serverId: string
): Promise<ServerBrandingAssetResponse> {
  const response = await csrfFetch(profileAssetUrl(serverId, `/api/server/${kind}`), {
    method: 'DELETE',
    headers: profileAssetHeaders(serverId)
  });
  if (!response.ok) {
    throw new Error(await profileAssetErrorMessage(response, `Server ${kind} delete failed`));
  }

  return ServerBrandingAssetResponse.fromBinary(new Uint8Array(await response.arrayBuffer()));
}

function requireServerId(serverId: string | null): string {
  if (!serverId) throw new Error('No active server');
  return serverId;
}

function profileAssetUrl(serverId: string, path: string): string {
  if (serverRegistry.isOriginServer(serverId)) {
    return path;
  }
  const server = serverRegistry.getServer(serverId);
  if (!server) throw new Error(`Server "${serverId}" not found`);
  return `${server.url.replace(/\/$/, '')}${path}`;
}

function profileAssetHeaders(serverId: string): Headers {
  const headers = new Headers({ Accept: PROTOBUF_CONTENT_TYPE });
  const token = serverRegistry.getServer(serverId)?.token;
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  return headers;
}

async function profileAssetErrorMessage(response: Response, message: string): Promise<string> {
  const fallback = `${message} (${response.status})`;
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
