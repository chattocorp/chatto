import '$lib/apiClientHooks';
import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '$lib/api-client/server';
import { preloadPublicLocaleMessages } from '$lib/i18n/messages';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import { segmentToServerId } from '$lib/navigation';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { loadSavedView } from '$lib/storage/savedViews';
import { getLastRoom } from '$lib/storage/lastRoom';
import type { SavedView } from '$lib/storage/savedViews';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

/** Only the first browser route load can bypass network work for a saved view. */
let initialLoad = true;

export const load: LayoutLoad = async ({ url, params }) => {
  const originHasBackend = isBackendCapableOrigin(url);
  const coldStart = initialLoad;
  if (coldStart) serverRegistry.init(true);
  const routeServerId = params?.serverId ? segmentToServerId(params.serverId) : null;
  const serverId = coldStart ? routeServerId : null;
  const savedUserId = serverId ? serverRegistry.getServer(serverId)?.userId ?? null : null;
  if (coldStart && params?.serverId && serverId && savedUserId &&
    url.pathname === resolve('/chat/[serverId]', { serverId: params.serverId })) {
    const lastRoomId = getLastRoom(serverId);
    const landing = lastRoomId
      ? resolve('/chat/[serverId]/[roomId]', { serverId: params.serverId, roomId: lastRoomId })
      : resolve('/chat/[serverId]/overview', { serverId: params.serverId });
    redirect(302, `${landing}${url.search}`);
  }
  initialLoad = false;
  let startupSavedView: SavedView | null = null;
  let publicLocalePromise: Promise<void> | null = null;
  // A message permalink may point outside the bounded saved window. Resolve
  // it against the live projection so startup replacement cannot lose its jump.
  if (serverId && savedUserId && !params?.messageId && !isExplicitSignOutRedirectInProgress()) {
    publicLocalePromise = preloadPublicLocaleMessages();
    const [view] = await Promise.all([
      loadSavedView(serverId, savedUserId),
      publicLocalePromise
    ]);
    if (!isExplicitSignOutRedirectInProgress() &&
      serverRegistry.getServer(serverId)?.userId === savedUserId) startupSavedView = view;
  }
  if (startupSavedView && serverId) {
    // Install the saved projection before any connection or viewer request.
    serverRegistry.init(true);
    serverRegistry.getStore(serverId).restoreSavedView(startupSavedView, true);
    return {
      serverInfo: null,
      serverInfoLoaded: false,
      user: null,
      startupServerId: serverId,
      startupPending: true
    };
  }
  // Initialise persisted remote sessions before child route loads read them.
  // This is idempotent across SPA navigations.
  const savedStartupServerId = routeServerId &&
    serverRegistry.tryGetStore(routeServerId)?.startupPresentationOnly ? routeServerId : null;
  serverRegistry.init(true);
  if (savedStartupServerId) serverRegistry.startServerNetwork(savedStartupServerId);
  const [, serverInfo, user] = await Promise.all([
    publicLocalePromise ?? preloadPublicLocaleMessages(),
    originHasBackend ? getPublicServerInfo(url.origin).catch(() => null) : null,
    originHasBackend ? loadCurrentUser() : null
  ]);

  // Child route loads need a settled origin registry to resolve the "-" URL
  // segment and make authentication decisions before components render.
  const retainSavedOrigin = serverRegistry.originServer?.id
    ? serverRegistry.tryGetStore(serverRegistry.originServer.id)?.startupPresentationOnly === true
    : false;
  await serverRegistry.probeOrigin(user !== null || retainSavedOrigin, undefined, serverInfo ?? undefined);
  if (!user && !retainSavedOrigin) serverRegistry.settleOriginUnauthenticated();

  if (serverInfo?.setupRequired && (url.pathname === '/' || url.pathname === '/login' || url.pathname.startsWith('/register'))) {
    redirect(302, '/setup');
  }

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
