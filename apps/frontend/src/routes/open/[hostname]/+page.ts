import { resolve } from '$app/paths';
import { redirect } from '@sveltejs/kit';
import { serverIdToSegment } from '$lib/navigation';
import { serverIdForHostname } from '$lib/navigation/serverHostname';
import { findServerByUrl } from '$lib/serverCatalogue';
import { handoffHostname } from '$lib/serverUrl';
import type { PageLoad } from './$types';

/** Resolve a saved server locally. Discovery of a new origin requires a user action. */
export const load: PageLoad = async ({ params, url, parent }) => {
  await parent();
  const hostname = url.search ? null : handoffHostname(params.hostname);
  if (!hostname) return { hostname: null };

  const saved = findServerByUrl(`https://${hostname}`);
  const segment = saved && serverIdToSegment(saved.id);
  // Remote chat routes use only the hostname. A different port or scheme can
  // already own this segment. In that case offer the exact HTTPS site.
  const routeOwner = serverIdForHostname(hostname);
  if (saved && segment && (segment === '-' || routeOwner === saved.id)) {
    redirect(302, resolve('/chat/[serverId]', { serverId: segment }));
  }
  return { hostname, external: !!routeOwner };
};
