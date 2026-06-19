/**
 * Authentication utilities for SvelteKit load functions.
 *
 * These functions can be used in +layout.ts and +page.ts files to check
 * authentication status and redirect unauthenticated users before components render.
 */

import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { browser } from '$app/environment';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { CurrentUserPresenceStatus, type CurrentUserView } from '$lib/pb/chatto/api/v1/chat_pb';
import { TimeFormat as WireTimeFormat } from '$lib/pb/chatto/core/v1/user_preferences_pb';
import { ErrorCode } from '$lib/pb/chatto/wire/v1/protocol_pb';
import { WireClient, WireProtocolError } from '$lib/wire/client';
import { PresenceStatus, TimeFormat } from '$lib/chatTypes';

export interface CurrentUser {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  presenceStatus: PresenceStatus;
  hasVerifiedEmail: boolean;
  settings: {
    timezone?: string | null;
    timeFormat: TimeFormat;
  } | null;
}

// Module-level cache for the current user. Root load re-checks the server on
// navigation, but keeps this value as a fallback when the check itself fails.
let cachedUser: CurrentUser | null = null;

/**
 * Load the current user from the protobuf wire API.
 * Returns null if not authenticated.
 *
 * On transient network errors (e.g., slow CI, server still warming up after reload),
 * retries once. If the wire request still fails and we previously had a user, keep the
 * cached user rather than rendering the app as unauthenticated.
 */
export async function loadCurrentUser(): Promise<CurrentUser | null> {
  if (!browser) {
    // In SPA mode, load functions only run in the browser.
    // If somehow called on server, return null (will trigger redirect).
    return null;
  }

  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      cachedUser = await fetchCurrentUserViaWire(
        serverConnectionManager.originClient.wireUrl,
        serverConnectionManager.originClient.token
      );
      return cachedUser;
    } catch (err) {
      if (isWireAuthenticationRequiredError(err)) {
        cachedUser = null;
        serverRegistry.clearOriginAuthentication();
        return null;
      }
      if (attempt === 0) {
        await new Promise((r) => setTimeout(r, 200));
        continue;
      }
      return cachedUser;
    }
  }

  return cachedUser;
}

export async function fetchCurrentUserViaWire(
  wireUrl: string,
  token: string | null
): Promise<CurrentUser | null> {
  const client = new WireClient({ url: wireUrl, token });
  try {
    const resp = await client.getCurrentUser();
    return currentUserFromWire(resp.user);
  } finally {
    client.dispose();
  }
}

export function currentUserFromWire(view: CurrentUserView | undefined): CurrentUser | null {
  const user = view?.user;
  if (!user) return null;
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: view.avatarUrl || null,
    presenceStatus: presenceStatusFromWire(view.presenceStatus),
    hasVerifiedEmail: view.hasVerifiedEmail,
    settings: {
      timezone: view.settings?.timezone ?? null,
      timeFormat: timeFormatFromWire(view.settings?.timeFormat)
    }
  };
}

export function isWireAuthenticationRequiredError(error: unknown): boolean {
  return error instanceof WireProtocolError && error.wireError?.code === ErrorCode.UNAUTHENTICATED;
}

function presenceStatusFromWire(status: CurrentUserPresenceStatus): PresenceStatus {
  switch (status) {
    case CurrentUserPresenceStatus.ONLINE:
      return PresenceStatus.Online;
    case CurrentUserPresenceStatus.AWAY:
      return PresenceStatus.Away;
    case CurrentUserPresenceStatus.DO_NOT_DISTURB:
      return PresenceStatus.DoNotDisturb;
    case CurrentUserPresenceStatus.OFFLINE:
    case CurrentUserPresenceStatus.UNSPECIFIED:
    default:
      return PresenceStatus.Offline;
  }
}

function timeFormatFromWire(format: WireTimeFormat | undefined): TimeFormat {
  switch (format) {
    case WireTimeFormat.TIME_FORMAT_12H:
      return TimeFormat.TwelveHour;
    case WireTimeFormat.TIME_FORMAT_24H:
      return TimeFormat.TwentyFourHour;
    case WireTimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

/**
 * Clear the cached user. Call this when the user logs out.
 */
export function clearCachedUser(): void {
  cachedUser = null;
}

/**
 * Require authentication in a load function.
 * If not authenticated, stores the return URL and redirects to the home page.
 *
 * @param returnUrl - The URL to return to after login. Stored in sessionStorage.
 * @returns The authenticated user.
 * @throws Redirect to '/' if not authenticated.
 *
 * @example
 * // In +layout.ts or +page.ts
 * export const load: LayoutLoad = async ({ url }) => {
 *   const user = await requireAuth(url.pathname + url.search);
 *   return { user };
 * };
 */
export async function requireAuth(returnUrl?: string): Promise<CurrentUser> {
  const user = await loadCurrentUser();
  return requireUser(user, returnUrl);
}

/**
 * Require that a user is authenticated, redirecting to home if not.
 * Use this when you already have the user from a parent load function.
 *
 * @param user - The user from parent layout data (may be null)
 * @param returnUrl - The URL to return to after login. Stored in sessionStorage.
 * @returns The authenticated user.
 * @throws Redirect to '/' if not authenticated.
 *
 * @example
 * // In +layout.ts or +page.ts
 * export const load: LayoutLoad = async ({ url, parent }) => {
 *   const { user } = await parent();
 *   return { user: requireUser(user, url.pathname + url.search) };
 * };
 */
export function requireUser(user: CurrentUser | null, returnUrl?: string): CurrentUser {
  if (!user) {
    if (returnUrl && browser) {
      sessionStorage.setItem('returnUrl', returnUrl);
    }
    redirect(302, resolve('/'));
  }

  return user;
}
