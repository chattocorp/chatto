import {
  beginExplicitSignOutRedirect,
  cancelExplicitSignOutRedirect,
  ServerLogoutRejectedError,
  signOutServer,
  signOutServers
} from '@chatto/client/auth/signOut';
import { notifyLogout } from '$lib/auth/sessionChannel';
import { clearLastRoom } from '$lib/storage/lastRoom';
import { serverRegistry } from '$lib/client';
import { firstAuthenticatedServerId } from '$lib/serverCatalogue';

/**
 * Removes a server's push delivery to this device before leaving it. Loaded on
 * demand, so Web Push registration code stays out of the initial bundle.
 */
async function unsubscribePushBeforeLeaving(serverId: string): Promise<void> {
  const { unsubscribeBeforeLeaving } = await import('$lib/notifications/pushNotifications');
  await unsubscribeBeforeLeaving(serverId);
}

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
        serverId: firstAuthenticatedServerId(serverId)
      };
    }

    serverRegistry.clearServerAuthentication(serverId);
    return {
      kind: 'soft',
      serverId: firstAuthenticatedServerId(serverId)
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
