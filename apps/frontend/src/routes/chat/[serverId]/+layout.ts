import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { saveReturnUrl } from '$lib/auth/returnNavigation';
import { segmentToServerId } from '$lib/navigation';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { loadSavedView } from '$lib/storage/savedViews';
import type { LayoutLoad } from './$types';

function redirectToLogin(url: URL): never {
  saveReturnUrl(url.pathname + url.search);
  return redirect(302, resolve('/login'));
}

export const load: LayoutLoad = async ({ params, parent, url }) => {
  const { user, serverInfo, startupServerId } = await parent();
  const serverId = segmentToServerId(params.serverId);
  const serverStore = serverId ? serverRegistry.tryGetStore(serverId) : undefined;

  if (!serverId || !serverStore) redirectToLogin(url);

  if (serverRegistry.isOriginServer(serverId) && serverInfo?.setupRequired) {
    redirect(302, resolve('/setup'));
  }

  const savedView = startupServerId === serverId
    ? serverStore.savedView
    : await loadSavedView(serverId, serverRegistry.getServer(serverId)?.userId ?? null);
  serverStore.restoreSavedView(savedView, serverStore.networkStartupDeferred);
  // A dormant server starts its network work after its saved view is ready.
  if (startupServerId !== serverId) serverRegistry.startServerNetwork(serverId);

  let reauthRequired = serverRegistry.getServer(serverId)?.reauthRequiredAt != null;
  if (!savedView && !reauthRequired && !serverRegistry.isOriginServer(serverId)) {
    // Registry initialisation begins remote viewer loading before route loads.
    // Await it only while the first request is still in flight.
    if (serverStore.currentUser.loading) await serverStore.currentUser.load();
  }

  // A failed remote viewer request can transition the session to the existing
  // reauthentication recovery state while it is awaited above.
  reauthRequired = serverRegistry.getServer(serverId)?.reauthRequiredAt != null;

  const authenticated = serverRegistry.isOriginServer(serverId)
    ? user !== null
    : serverStore.currentUser.user !== undefined;
  if (!reauthRequired && !authenticated && !savedView) redirectToLogin(url);

  return {
    serverSegment: params.serverId,

    /** The currently active room (from child route params). */
    roomId: params.roomId
  };
};
