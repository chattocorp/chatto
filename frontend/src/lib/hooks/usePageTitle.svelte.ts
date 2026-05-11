/**
 * Computes the full page title including notification count badge.
 * Call during component initialization — returns a reactive getter.
 */

import { titleState } from '$lib/state/globals.svelte';
import { instanceRegistry } from '$lib/state/instance/registry.svelte';

export function usePageTitle(): () => string {
  const fullTitle = $derived.by(() => {
    const origin = instanceRegistry.originInstance;
    const serverName = origin
      ? (instanceRegistry.getStore(origin.id).instance.name || 'Chatto')
      : 'Chatto';
    const base = titleState.pageTitle
      ? `${titleState.pageTitle} | ${serverName}`
      : serverName;

    const totalCount = instanceRegistry.instances.reduce((sum, instance) => {
      const store = instanceRegistry.getStore(instance.id);
      if (!store.isAuthenticated) return sum;
      return sum + store.notifications.count;
    }, 0);

    return totalCount > 0 ? `(${totalCount}) ${base}` : base;
  });

  return () => fullTitle;
}
