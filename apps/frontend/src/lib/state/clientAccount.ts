import { beginExplicitSignOutRedirect, signOutServer, signOutServers } from '$lib/auth/signOut';
import { notifyLogout } from '$lib/auth/sessionChannel';
import { unsubscribeBeforeLeaving as unsubscribePushBeforeLeaving } from '$lib/notifications/pushNotifications';
import { clearLastRoom } from '$lib/storage/lastRoom';
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
    await signOutServer(server, origin).catch(() => undefined);
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
    notifyLogout();
    return { kind: 'hard' };
  }
}

export const clientAccount = new ClientAccountCoordinator();
