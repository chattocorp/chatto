/**
 * The frontend's sign-out command for one server. The Server Gutter menu, the
 * sidebar server menu, and the settings navigation share it, so each shows the
 * same progress state and lands on the same page afterwards.
 */

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { SvelteSet } from 'svelte/reactivity';
import { hardRedirectAfterSignOut } from '$lib/auth/signOutRedirect';
import { m } from '$lib/i18n/messages';
import { serverIdToSegment } from '$lib/navigation';
import { clientAccount } from '$lib/state/clientAccount';
import { toast } from '$lib/ui/toast';

/** Servers with a sign-out in progress in this tab. */
const signingOut = new SvelteSet<string>();

/** Whether this tab is signing out of `serverId`. Reactive. */
export function isSigningOut(serverId: string): boolean {
  return signingOut.has(serverId);
}

/**
 * Sign out of `serverId` and open the page that the client account selects.
 * A second call while one runs does nothing. A failure shows a toast.
 */
export async function signOutOfServer(serverId: string): Promise<void> {
  if (signingOut.has(serverId)) return;
  signingOut.add(serverId);
  try {
    const navigation = await clientAccount.signOutCurrentServer(serverId);
    if (!navigation) return;
    const href = navigation.serverId
      ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(navigation.serverId) })
      : resolve('/');
    if (navigation.kind === 'hard') {
      hardRedirectAfterSignOut(href);
    } else {
      await goto(href);
    }
  } catch {
    toast.error(m('common.error.network'));
  } finally {
    signingOut.delete(serverId);
  }
}
