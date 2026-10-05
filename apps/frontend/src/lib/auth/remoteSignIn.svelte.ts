import { SvelteSet } from 'svelte/reactivity';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { startRemoteReauthentication } from '$lib/auth/reauth';
import { m } from '$lib/i18n/messages';
import { toast } from '$lib/ui/toast';

/** IDs of the remote servers whose sign-in runs. */
const pendingServerIds = new SvelteSet<string>();

/** Whether sign-in to the server runs. Reactive in Svelte templates. */
export function isRemoteSignInPending(serverId: string): boolean {
  return pendingServerIds.has(serverId);
}

/**
 * The **Log in to this server** action for a registered remote server. The
 * gutter menu, the signed-out view, and the reconnect notice share it, so only
 * one sign-in for a server runs at a time. Shows a toast when sign-in fails.
 * Call this synchronously from the user's action, so the browser allows the
 * sign-in window.
 */
export async function startRemoteSignIn(server: RegisteredServer): Promise<void> {
  if (pendingServerIds.has(server.id)) return;
  pendingServerIds.add(server.id);
  try {
    await startRemoteReauthentication(server);
  } catch {
    toast.error(m('add_server.start_failed'));
  } finally {
    pendingServerIds.delete(server.id);
  }
}
