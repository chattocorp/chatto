import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import { browserCookieAuthenticationHeaders } from './authenticationMode';
import { csrfFetch } from './csrf';

export const SIGN_OUT_TIMEOUT_MS = 5000;

export class ServerLogoutRejectedError extends Error {
  constructor(readonly status: number) {
    super(`Server rejected logout with HTTP ${status}`);
    this.name = 'ServerLogoutRejectedError';
  }
}

let explicitSignOutRedirectInProgress = false;

function logoutUrl(server: RegisteredServer): string {
  return new URL('/auth/logout', server.url).toString();
}

function withSignOutTimeout<T>(request: (signal: AbortSignal) => Promise<T>): Promise<T> {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), SIGN_OUT_TIMEOUT_MS);
  return request(controller.signal).finally(() => clearTimeout(timeoutId));
}

/**
 * Requests server-side logout for a registered server.
 *
 * The promise rejects for transport failures and non-success responses. Each
 * caller decides whether it can safely continue with local cleanup.
 */
export async function signOutServer(
  server: RegisteredServer,
  isOriginServer: boolean
): Promise<Response> {
  const headers: Record<string, string> = {};
  if (server.token) headers.Authorization = `Bearer ${server.token}`;
  if (server.refreshToken) headers['Content-Type'] = 'application/json';
  const body = server.refreshToken
    ? JSON.stringify({ refreshToken: server.refreshToken })
    : undefined;

  if (isOriginServer) {
    headers['Content-Type'] = 'application/json';
    Object.assign(headers, browserCookieAuthenticationHeaders);
    const response = await withSignOutTimeout((signal) =>
      csrfFetch('/auth/browser/logout', {
        method: 'POST',
        headers,
        body: body ?? '{}',
        signal
      })
    );
    if (!response.ok) throw new ServerLogoutRejectedError(response.status);
    return response;
  }

  const response = await withSignOutTimeout((signal) =>
    fetch(logoutUrl(server), {
      method: 'POST',
      headers,
      body,
      signal
    })
  );
  if (!response.ok) throw new ServerLogoutRejectedError(response.status);
  return response;
}

export async function signOutServers(
  servers: RegisteredServer[],
  isOriginServer: (serverId: string) => boolean
): Promise<void> {
  await Promise.all(
    servers.map((server) => signOutServer(server, isOriginServer(server.id)).catch(() => undefined))
  );
}

export function isExplicitSignOutRedirectInProgress(): boolean {
  return explicitSignOutRedirectInProgress;
}

export function beginExplicitSignOutRedirect(): void {
  explicitSignOutRedirectInProgress = true;
}

export function cancelExplicitSignOutRedirect(): void {
  explicitSignOutRedirectInProgress = false;
}

export function hardRedirectAfterSignOut(href = '/'): void {
  beginExplicitSignOutRedirect();
  try {
    const target = new URL(href, window.location.href);
    if (target.origin === window.location.origin) {
      window.setTimeout(() => {
        window.location.replace(target.pathname + target.search + target.hash);
      }, 0);
      return;
    }
  } catch {
    // Fall back to a regular document navigation below.
  }
  window.setTimeout(() => {
    window.location.replace(href);
  }, 0);
}
