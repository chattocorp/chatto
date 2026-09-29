/**
 * The client runtime: background work that keeps every server of one client
 * current.
 *
 * It retries discovery and saved-session recovery, and it assigns each
 * authenticated server's realtime transport a mode: live (a persistent
 * WebSocket) or polling (see `realtimeTransport`). A remote server that
 * terminates its session is signed out locally.
 *
 * A client starts one runtime; see `ChattoClient.start`. An application that
 * shows one server at a time reports it with
 * {@link ClientRuntime.setActiveServer}.
 */

import { effect, effectRoot, signal, untrack } from '../reactivity/index.js';
import type { EventBusManager, RealtimeServerRegistration } from './realtimeTransport.js';
import type { ServerRegistry } from './registry.js';
import type { ServerConnectionManager } from './serverConnection.js';
import { startServerRecovery } from './serverRecovery.js';

/** The parts of a client that its runtime drives. */
export interface ClientRuntimeParts {
  readonly registry: ServerRegistry;
  readonly connections: ServerConnectionManager;
  readonly realtime: EventBusManager;
}

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
function realtimeRegistrations({
  registry,
  connections
}: ClientRuntimeParts): RealtimeServerRegistration[] {
  return registry.servers.flatMap((server) => {
    const store = registry.tryGetStore(server.id);
    return store?.isAuthenticated
      ? [
          {
            serverId: server.id,
            connection: connections.getClient(server.id),
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
export function startClientRuntime(parts: ClientRuntimeParts): ClientRuntime {
  const { registry, realtime } = parts;
  const activeServerId = signal<string | null>(null);
  let stopped = false;

  const disposeEffects = effectRoot(() => {
    // Registry and store changes often come in several writes, such as a
    // session replacement. Synchronize once after they settle, from the
    // latest inputs, as a component effect would.
    let pending: { registrations: RealtimeServerRegistration[]; active: string | null } | null =
      null;
    effect(() => {
      const next = { registrations: realtimeRegistrations(parts), active: activeServerId.get() };
      const scheduled = pending !== null;
      pending = next;
      if (scheduled) return;
      queueMicrotask(() => {
        const inputs = pending;
        pending = null;
        if (stopped || !inputs) return;
        // Synchronization changes connection state; it must not track reads.
        untrack(() =>
          realtime.synchronizeAuthenticatedServers(inputs.registrations, inputs.active)
        );
      });
    });

    // Remote session termination is authoritative even when its server is not
    // active: sign out that server. A fixed token, such as a bot API key,
    // cannot sign in again, so its session ends and its host is told. Reading
    // each bus subscribes again when a server's bus starts later.
    effect(() => {
      const remoteBuses = registry.servers
        .filter((server) => !registry.isOriginServer(server.id))
        .map((server) => ({ id: server.id, bus: realtime.getBus(server.id) }));
      return untrack(() => {
        const disposers = remoteBuses.map(({ id, bus }) =>
          bus?.onSessionTerminated(() => {
            queueMicrotask(() => {
              if (registry.hasFixedToken(id)) registry.handleAuthenticationRequired(id);
              else registry.clearServerAuthentication(id);
            });
          })
        );
        return () => disposers.forEach((dispose) => dispose?.());
      });
    });
  });
  // A failed start leaves neither effects nor a recovery timer behind.
  let stopRecovery: () => void;
  try {
    stopRecovery = startServerRecovery(registry);
  } catch (error) {
    stopped = true;
    disposeEffects();
    throw error;
  }

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
