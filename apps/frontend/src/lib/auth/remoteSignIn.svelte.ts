import { popupErrorMessage } from '$lib/oauth/popupError';
import { SvelteSet } from 'svelte/reactivity';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { cancelServerOAuthFlow, startRemoteReauthentication } from '$lib/auth/reauth';
import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { serverIdToSegment } from '$lib/navigation';
import { hasPasswordLogin } from '$lib/desktop/passwordLogin';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { invalidateAll } from '$app/navigation';
import { persistedBearerSession } from '@chatto/client/auth/bearerSession';
import { serverRegistry } from '$lib/client';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';

/** Explicit reauthentication requests can replace a retained chat view with the login form. */
const inlineServerIds = new SvelteSet<string>();

/** Whether an explicit Desktop sign-in should replace this server's retained view. */
export function isInlineSignInRequested(serverId: string): boolean {
  return hasPasswordLogin() && inlineServerIds.has(serverId);
}

/** Close an explicit inline login without clearing the existing session. */
export function closeInlineSignIn(serverId: string): void {
  inlineServerIds.delete(serverId);
}

/** IDs of the remote servers whose sign-in runs. */
const pendingServerIds = new SvelteSet<string>();

/** Whether sign-in to the server runs. Reactive in Svelte templates. */
export function isRemoteSignInPending(serverId: string): boolean {
  return pendingServerIds.has(serverId);
}

/** Stop waiting for the pending sign-in window of `server`. */
export function cancelRemoteSignIn(server: RegisteredServer): void {
  cancelServerOAuthFlow(server.url);
}

/**
 * The **Log in to server** action for a registered remote server. The
 * gutter menu, the signed-out view, and the reconnect notice share it, so only
 * one sign-in for a server runs at a time. Shows a toast when sign-in fails.
 * Call this synchronously from the user's action, so the browser allows the
 * sign-in window. Desktop shows an inline form unless `useOAuth` or `providerId` requests the
 * server-owned page for providers, registration, or recovery.
 */
export async function startRemoteSignIn(
  server: RegisteredServer,
  options: { useOAuth?: boolean; providerId?: string; signup?: boolean } = {}
): Promise<void> {
  if (hasPasswordLogin() && !options.useOAuth && !options.providerId && !options.signup) {
    inlineServerIds.add(server.id);
    if (getActiveServer() !== server.id) {
      try {
        await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(server.id) }));
      } catch {
        closeInlineSignIn(server.id);
        toast.error(m('add_server.start_failed'));
      }
    }
    return;
  }
  if (pendingServerIds.has(server.id)) return;
  pendingServerIds.add(server.id);
  try {
    if (options.signup) await startRemoteReauthentication(server, undefined, 'signup');
    else if (options.providerId) await startRemoteReauthentication(server, options.providerId);
    else await startRemoteReauthentication(server);
    closeInlineSignIn(server.id);
  } catch (error) {
    const message = popupErrorMessage(error, m('add_server.start_failed'));
    if (message) toast.error(message);
  } finally {
    pendingServerIds.delete(server.id);
  }
}

/**
 * Sign in to `server` with the device's AT Protocol identity (FDR-056).
 * `identifier` selects the identity; empty uses the device session. Without
 * an account for that identity, this opens the server's own sign-in window
 * for provider `providerId`, where the user can create one.
 */
export async function signInRemoteWithAtproto(
  server: RegisteredServer,
  providerId: string,
  identifier = ''
): Promise<void> {
  if (pendingServerIds.has(server.id)) return;
  pendingServerIds.add(server.id);
  let fallBack = false;
  try {
    const { signInToRemoteWithAtproto } = await import('$lib/atproto/signIn');
    const store = serverRegistry.tryGetStore(server.id);
    const credentials = await signInToRemoteWithAtproto(server.url, identifier);
    if (!credentials) {
      fallBack = true;
      return;
    }
    if (serverRegistry.tryGetStore(server.id) !== store || !serverRegistry.getServer(server.id)) {
      return;
    }
    serverRegistry.replaceServerAuthentication(server.id, {
      ...persistedBearerSession(credentials),
      userId: null,
      userLogin: null,
      userDisplayName: null,
      userAvatarUrl: null,
      reauthRequiredAt: null
    });
    resumePushRegistrationAfterAuthentication(server.id);
    closeInlineSignIn(server.id);
    await invalidateAll();
  } catch {
    toast.error(m('add_server.start_failed'));
  } finally {
    pendingServerIds.delete(server.id);
  }
  if (fallBack) await startRemoteSignIn(server, { useOAuth: true, providerId });
}
