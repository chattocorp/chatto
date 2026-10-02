import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { saveReturnUrl } from '$lib/auth/returnNavigation';
import { segmentToServerId } from '$lib/navigation';
import { serverRegistry } from '$lib/client';
import type { LayoutLoad } from './$types';

function redirectToLogin(url: URL): never {
  saveReturnUrl(url.pathname + url.search);
  return redirect(302, resolve('/login'));
}

export const load: LayoutLoad = async ({ params, parent, url }) => {
  const { user, serverInfo } = await parent();
  const serverId = segmentToServerId(params.serverId);
  const serverStore = serverId ? serverRegistry.tryGetStore(serverId) : undefined;

  if (!serverId || !serverStore) redirectToLogin(url);

  if (serverRegistry.isOriginServer(serverId) && serverInfo?.setupRequired) {
    redirect(302, resolve('/setup'));
  }

  if (serverRegistry.isOriginServer(serverId)) {
    // `/login` is the origin's own sign-in page. Reauthentication recovery
    // keeps the shell mounted and shows its reconnect notice instead.
    const reauthRequired = serverRegistry.getServer(serverId)?.reauthRequiredAt != null;
    if (!reauthRequired && user === null) redirectToLogin(url);
  } else if (serverStore.currentUser.loading) {
    // Registry initialisation begins remote viewer loading before route loads.
    // Await the first request so the layout does not show a signed-out or
    // empty view only for a moment. A remote server never redirects to the
    // origin's `/login`: the layout shows a view for each of its states.
    await serverStore.currentUser.load();
  }

  // Do not read child params here. SvelteKit re-runs a load when a param that
  // it read changes, and this load checks access.
  return {
    serverSegment: params.serverId
  };
};
