import { redirect } from '@sveltejs/kit';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '@chatto/client/api/server';
import { preloadPublicLocaleMessages } from '$lib/i18n/messages';
import { isBackendCapableOrigin } from '@chatto/client/util/runtimeOrigin';
import { serverRegistry } from '$lib/client';
import { registerOriginServer } from '$lib/serverCatalogue';
import { deleteLegacySavedViews } from '$lib/storage/legacySavedViews';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

let initialLoad = true;

export const load: LayoutLoad = async ({ url }) => {
  if (initialLoad) {
    initialLoad = false;
    deleteLegacySavedViews();
  }
  const originHasBackend = isBackendCapableOrigin(url);
  // Initialise persisted remote sessions before child route loads read them.
  // This is idempotent across SPA navigations.
  serverRegistry.init();
  const serverInfoPromise = originHasBackend
    ? getPublicServerInfo(url.origin).catch(() => null)
    : Promise.resolve(null);
  const userPromise = originHasBackend
    ? (async () => {
        if (!serverRegistry.originServer) {
          const info = await serverInfoPromise;
          if (!info) return null;
          await registerOriginServer({ serverInfo: info });
        }
        return loadCurrentUser(serverRegistry);
      })()
    : Promise.resolve(null);
  const [, serverInfo, user] = await Promise.all([
    preloadPublicLocaleMessages(),
    serverInfoPromise,
    userPromise
  ]);

  // Child route loads need a settled origin registry to resolve the "-" URL
  // segment and make authentication decisions before components render.
  await registerOriginServer({ signedIn: user !== null, serverInfo });

  if (
    serverInfo?.setupRequired &&
    (url.pathname === '/' || url.pathname === '/login' || url.pathname.startsWith('/register'))
  ) {
    redirect(302, '/setup');
  }

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
