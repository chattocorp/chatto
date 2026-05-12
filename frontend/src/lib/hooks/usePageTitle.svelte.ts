/**
 * Computes the full page title including notification count badge.
 * Call during component initialization — returns a reactive getter.
 */

import { titleState } from '$lib/state/globals.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export function usePageTitle(): () => string {
  const fullTitle = $derived.by(() => {
    const origin = serverRegistry.originServer;
    const serverName = origin
      ? (serverRegistry.getStore(origin.id).serverInfo.name || 'Chatto')
      : 'Chatto';
    const base = titleState.pageTitle
      ? `${titleState.pageTitle} | ${serverName}`
      : serverName;

    const totalCount = serverRegistry.instances.reduce((sum, instance) => {
      const store = serverRegistry.getStore(instance.id);
      if (!store.isAuthenticated) return sum;
      return sum + store.notifications.count;
    }, 0);

    return totalCount > 0 ? `(${totalCount}) ${base}` : base;
  });

  return () => fullTitle;
}
