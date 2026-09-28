/**
 * The client runtime: background work that keeps every registered server's
 * state current.
 *
 * It retries discovery and saved-session recovery, and it assigns each
 * authenticated server's realtime transport a mode. Only the active server
 * keeps a persistent WebSocket; other servers catch up by polling (see
 * `realtimeTransport`). A remote server that terminates its session is signed
 * out locally.
 *
 * An application starts one runtime for its lifetime and reports the server
 * that the user is looking at with {@link ClientRuntime.setActiveServer}.
 */

import { effect, effectRoot, signal, untrack } from '../reactivity/index.js';
import { eventBusManager, type RealtimeServerRegistration } from './realtimeTransport.js';
import { serverRegistry } from './registry.js';
import { serverConnectionManager } from './serverConnection.js';
import { startServerRecovery } from './serverRecovery.js';

/** A running client runtime. */
export interface ClientRuntime {
  /**
   * Select the server whose realtime transport stays live, or null for none.
   * A server that is not authenticated is ignored until it is.
   */
  setActiveServer(serverId: string | null): void;
  /** Stop background work. Realtime transports stay as they are. */
  stop(): void;
}

/** Realtime registrations for every authenticated server, read reactively. */
function realtimeRegistrations(): RealtimeServerRegistration[] {
  return serverRegistry.servers.flatMap((server) => {
    const store = serverRegistry.tryGetStore(server.id);
    return store?.isAuthenticated
      ? [
          {
            serverId: server.id,
            connection: serverConnectionManager.getClient(server.id),
            projectionSupported: store.serverInfo.isSupportedVersion,
            sync: store.realtimeSync,
            projectionHandler: store.realtimeProjectionHandler,
            completeProjectionCatchUp: (cursor: string) => store.completeRealtimeCatchUp(cursor),
            waitForProjectionReconciliation: () => store.waitForRealtimeReconciliation()
          }
        ]
      : [];
  });
}

/** Start recovery, realtime ownership, and session-termination handling. */
export function startClientRuntime(): ClientRuntime {
  const activeServerId = signal<string | null>(null);
  const stopRecovery = startServerRecovery(serverRegistry);
  let stopped = false;

  const disposeEffects = effectRoot(() => {
    // Registry and store changes often come in several writes, such as a
    // session replacement. Synchronize once after they settle, from the
    // latest inputs, as a component effect would.
    let pending: { registrations: RealtimeServerRegistration[]; active: string | null } | null =
      null;
    effect(() => {
      const next = { registrations: realtimeRegistrations(), active: activeServerId.get() };
      const scheduled = pending !== null;
      pending = next;
      if (scheduled) return;
      queueMicrotask(() => {
        const inputs = pending;
        pending = null;
        if (stopped || !inputs) return;
        // Synchronization changes connection state; it must not track reads.
        untrack(() =>
          eventBusManager.synchronizeAuthenticatedServers(inputs.registrations, inputs.active)
        );
      });
    });

    // Remote session termination is authoritative even when its server is not
    // active: sign out that server. A fixed token, such as a bot API key,
    // cannot sign in again, so its session ends and its host is told. Reading
    // each bus subscribes again when a server's bus starts later.
    effect(() => {
      const remoteBuses = serverRegistry.servers
        .filter((server) => !serverRegistry.isOriginServer(server.id))
        .map((server) => ({ id: server.id, bus: eventBusManager.getBus(server.id) }));
      return untrack(() => {
        const disposers = remoteBuses.map(({ id, bus }) =>
          bus?.onSessionTerminated(() => {
            queueMicrotask(() => {
              if (serverRegistry.hasFixedToken(id)) serverRegistry.handleAuthenticationRequired(id);
              else serverRegistry.clearServerAuthentication(id);
            });
          })
        );
        return () => disposers.forEach((dispose) => dispose?.());
      });
    });
  });

  return {
    setActiveServer(serverId) {
      activeServerId.set(serverId);
    },
    stop() {
      if (stopped) return;
      stopped = true;
      disposeEffects();
      stopRecovery();
    }
  };
}
