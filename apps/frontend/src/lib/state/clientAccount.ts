import {
  beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect,
  ServerLogoutRejectedError,
  signOutServer,
  signOutServers
} from '$lib/auth/signOut';
import { notifyLogout } from '$lib/auth/sessionChannel';
import { unsubscribeBeforeLeaving as unsubscribePushBeforeLeaving } from '$lib/notifications/pushNotifications';
import { clearLastRoom } from '$lib/storage/lastRoom';
import { clearAllSavedViews } from '$lib/storage/savedViews';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export interface ClientAccountNavigation {
  kind: 'hard' | 'soft';
  serverId?: string;
}

/** Coordinates user commands that cross the device-local catalogue and sessions. */
class ClientAccountCoordinator {
  async signOutCurrentServer(serverId: string): Promise<ClientAccountNavigation | null> {
    const server = serverRegistry.getServer(serverId);
    if (!server) return null;

    const origin = serverRegistry.isOriginServer(serverId);
    await unsubscribePushBeforeLeaving(serverId);
    if (origin) beginExplicitSignOutRedirect();
    try {
      await signOutServer(server, origin);
    } catch (error) {
      if (error instanceof ServerLogoutRejectedError) {
        if (origin) cancelExplicitSignOutRedirect();
        throw error;
      }
      // A client can still discard local state when the server is unreachable.
    }
    clearLastRoom(serverId);

    if (origin) {
      serverRegistry.clearServerAuthentication(serverId);
      notifyLogout();
      return {
        kind: 'hard',
        serverId: serverRegistry.firstAuthenticatedServerId(serverId)
      };
    }

    serverRegistry.clearServerAuthentication(serverId);
    return {
      kind: 'soft',
      serverId: serverRegistry.firstAuthenticatedServerId(serverId)
    };
  }

  async signOutAllServers(): Promise<ClientAccountNavigation> {
    for (const server of serverRegistry.servers) {
      await unsubscribePushBeforeLeaving(server.id);
    }
    beginExplicitSignOutRedirect();
    await signOutServers([...serverRegistry.servers], (serverId) =>
      serverRegistry.isOriginServer(serverId)
    );
    serverRegistry.resetToOrigin();
    // Delete all device databases before the hard redirect unloads this page.
    // This also removes a damaged or newer database that normal reads reject.
    await clearAllSavedViews({ allDatabases: true, timeoutMs: 2_000 });
    notifyLogout();
    return { kind: 'hard' };
  }
}

export const clientAccount = new ClientAccountCoordinator();
