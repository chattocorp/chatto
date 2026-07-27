import { serverRegistry } from './registry.svelte';
import { registerServerResumeCallback } from '$lib/hooks/resumeCoordinator.svelte';
import type { ViewerState } from '$lib/api-client/viewer';

const SERVER_INFO_RESUME_REFRESH_MIN_MS = 60_000;

/**
 * Bootstrap the server registry: create stores, probe the origin,
 * and re-fetch server info when the tab resumes.
 *
 * Must be called during component initialization (root layout script).
 *
 * @param getViewer - Getter returning the origin viewer (truthy = known server,
 *   falsy = probe needed). Passed as a getter so reads happen inside `$effect`.
 */
export function useServerRegistry(getViewer: () => ViewerState | null): void {
  serverRegistry.init();
  const viewer = getViewer();
  const hasUser = !!viewer;
  serverRegistry.probeOrigin(hasUser);
  if (viewer) {
    serverRegistry.seedOriginViewer(viewer);
  }
  if (!hasUser) {
    serverRegistry.settleOriginUnauthenticated();
  }
  let seededViewer = viewer;

  // Keep the long-lived root layout's store synchronized when SvelteKit
  // produces a new viewer snapshot on a later navigation.
  $effect(() => {
    const nextViewer = getViewer();
    if (nextViewer === seededViewer) return;
    seededViewer = nextViewer;
    if (nextViewer) {
      serverRegistry.seedOriginViewer(nextViewer);
    } else {
      serverRegistry.settleOriginUnauthenticated();
    }
  });

  // Re-fetch server info after meaningful tab resumes. Quick tab switches do
  // not need another metadata/settings round trip.
  $effect(() => {
    const originId = serverRegistry.originServer?.id;
    if (!originId) return;
    let lastResumeRefreshAt = Date.now();

    return registerServerResumeCallback(originId, (signal) => {
      const now = Date.now();
      if (
        (signal.hiddenDurationMs ?? 0) < 30_000 &&
        now - lastResumeRefreshAt < SERVER_INFO_RESUME_REFRESH_MIN_MS
      ) {
        return;
      }
      lastResumeRefreshAt = now;

      const store = serverRegistry.getStore(originId);
      void store.serverInfo.init();
      if (store.isAuthenticated) {
        store.serverInfo.refreshAuthenticatedSettings().catch((err) => {
          console.error(
            `[server:${store.serverId}] failed to refresh authenticated server settings`,
            err
          );
        });
      }
    });
  });
}
