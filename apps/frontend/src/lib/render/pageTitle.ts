/**
 * Composes page, route identity, and important unread notifications for PageTitle.
 */

import { serverRegistry } from '$lib/state/server/registry.svelte';
import { segmentToServerId } from '$lib/navigation';
import { page } from '$app/state';

/** Call in a reactive expression to track route, server name, and unread count changes. */
export function formatPageTitle(title = '', scope: 'route' | 'app' = 'route'): string {
  // App-wide chat pages have no server segment. Public pages retain origin branding.
  const appScoped =
    scope === 'app' ||
    (!page.params.serverId && (page.route.id === '/chat' || page.route.id?.startsWith('/chat/')));
  const serverId = appScoped ? null : segmentToServerId(page.params.serverId ?? '-');
  const serverName =
    (serverId && serverRegistry.tryGetStore(serverId)?.serverInfo.name) || 'Chatto';
  const base = title ? `${title} · ${serverName}` : serverName;

  const totalCount = serverRegistry.servers.reduce((sum, instance) => {
    const store = serverRegistry.tryGetStore(instance.id);
    if (!store?.isAuthenticated) return sum;
    return sum + store.notifications.attention.importantUnreadNotificationCount;
  }, 0);

  return totalCount > 0 ? `(${totalCount}) ${base}` : base;
}
