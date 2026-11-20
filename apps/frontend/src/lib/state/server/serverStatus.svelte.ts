/**
 * Status of one registered server as the server menus show it: whether the
 * user must sign in, whether recovery can help, which server actions apply,
 * and the problem to report. The Server Gutter icon and the server menus read
 * the same values, so they cannot disagree.
 */

import { page } from '$app/state';
import { serverConnectionManager, serverRegistry } from '$lib/client';
import { m } from '$lib/i18n/messages';
import { serverHost } from '$lib/serverUrl';
import { MINIMUM_CONNECTABLE_SERVER_VERSION } from '@chatto/client/server/compatibility';
import { serverDisplayName } from '@chatto/client/server/state';

/** Marker on the server icon. A limited but working server has no marker. */
export type ServerStatusBadge = 'sign-in' | 'offline' | 'compatibility' | 'incompatible';

/** Reactive status of `serverId`. Read its getters in reactive contexts. */
export type ServerStatus = ReturnType<typeof serverStatus>;

/** Create the status view of a registered server with a store. */
export function serverStatus(serverId: string) {
  const stores = serverRegistry.getStore(serverId);
  const connection = serverConnectionManager.getClient(serverId);

  const registeredServer = () => serverRegistry.getServer(serverId);
  const isOrigin = () => serverRegistry.isOriginServer(serverId);
  // Setup belongs to the origin server, while the client remains multi-server.
  const setupRequired = () => isOrigin() && page.data.serverInfo?.setupRequired === true;
  const privateDataLoaded = () => stores.projection?.viewer != null;
  const needsReauth = () => registeredServer()?.reauthRequiredAt != null;
  const signInRequired = () => {
    if (setupRequired()) return false;
    const needsSignIn = !stores.isAuthenticated && (!registeredServer()?.token || needsReauth());
    return needsSignIn || needsReauth();
  };
  const name = () => serverDisplayName(stores.serverInfo, registeredServer()?.name);

  return {
    /** The registered server's ID. */
    serverId,
    /** The server's stores. */
    stores,
    /** The server's connection. */
    connection,
    /** Whether this server serves the frontend. */
    get isOrigin() {
      return isOrigin();
    },
    /** The registry entry, or `undefined` after removal. */
    get registeredServer() {
      return registeredServer();
    },
    /** Display name of the server. */
    get name() {
      return name();
    },
    /** Host name of the server URL, or `null` without a registry entry. */
    get host() {
      const server = registeredServer();
      return server ? serverHost(server.url) : null;
    },
    /** Whether the origin waits for its first-run setup. */
    get setupRequired() {
      return setupRequired();
    },
    /** Whether the server's private data is loaded for the viewer. */
    get privateDataLoaded() {
      return privateDataLoaded();
    },
    /** Whether the user must sign in (again) to use this server. */
    get signInRequired() {
      return signInRequired();
    },
    /** Whether a retry can help, for example after the server was unreachable. */
    get recoveryNeeded() {
      return (
        stores.serverInfo.compatibility.status === 'unreachable' ||
        serverRegistry.needsRecovery(serverId)
      );
    },
    /** Whether actions on the user's server data, such as Mark as read, can run. */
    get serverActionsAvailable() {
      return (
        stores.isAuthenticated &&
        privateDataLoaded() &&
        stores.serverInfo.canConnect &&
        !connection.showConnectionLostIcon
      );
    },
    /**
     * A connection problem or compatibility warning, or `null`. Discovery and
     * connection attempts in progress are not problems; only a failed attempt is.
     */
    get problem(): { badge: ServerStatusBadge | null; message: string } | null {
      // A login or retry cannot resolve a confirmed incompatibility.
      if (stores.serverInfo.blockingCompatibilityProblem === 'server-too-old') {
        return {
          badge: 'incompatible',
          message: m('chat.server_gutter.compatibility_server_too_old', {
            version: MINIMUM_CONNECTABLE_SERVER_VERSION
          })
        };
      }
      if (connection.realtimeUnsupported) {
        return { badge: 'incompatible', message: m('chat.server_gutter.protocol_unsupported') };
      }
      if (signInRequired()) {
        return {
          badge: 'sign-in',
          message: m('ui.auth_status.sidebar_reauth', { server: name() })
        };
      }
      const compatibility = stores.serverInfo.compatibility;
      if (!stores.serverInfo.loading && compatibility.reason === 'unreachable') {
        return { badge: 'offline', message: m('chat.server_gutter.unreachable') };
      }
      if (connection.showConnectionLostIcon) {
        return { badge: 'offline', message: m('chat.server_gutter.connection_unavailable') };
      }
      if (stores.serverInfo.loading) return null;
      switch (compatibility.reason) {
        case 'server-too-old':
          return { badge: null, message: m('chat.compatibility.limited') };
        case 'server-version-unknown':
          return { badge: 'compatibility', message: m('chat.server_gutter.compatibility_unknown') };
      }
      return null;
    }
  };
}
