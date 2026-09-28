/** A `ChattoConnection` test double backed by an in-memory Connect router. */

import { createClient, createRouterTransport, type ConnectRouter } from '@connectrpc/connect';
import type { ServiceType } from '@bufbuild/protobuf';
import type { ChattoConnection, ChattoConnectionStatus, RealtimeEvent } from '@chatto/client';
import { signal } from '@chatto/client/reactivity';

export function fakeConnection(
  routes: (router: ConnectRouter) => void = () => {},
  identity: Promise<{ viewerId: string }> = Promise.resolve({ viewerId: 'bot' })
) {
  const transport = createRouterTransport(routes);
  const eventListeners = new Set<(event: RealtimeEvent) => void>();
  const resetListeners = new Set<(reset: { gap: boolean }) => void>();
  const status = signal<ChattoConnectionStatus>('connecting');
  const sessionEnded = signal(false);
  const closed = signal(false);
  const connection = {
    serverId: 'test',
    connection: {
      get status() {
        return status.get();
      }
    },
    get sessionEnded() {
      return sessionEnded.get();
    },
    get closed() {
      return closed.get();
    },
    async ready({ signal }: { signal?: AbortSignal } = {}) {
      signal?.throwIfAborted();
      return identity;
    },
    service<T extends ServiceType>(service: T) {
      return createClient(service, transport);
    },
    onEvent(listener: (event: RealtimeEvent) => void) {
      eventListeners.add(listener);
      return () => eventListeners.delete(listener);
    },
    onReset(listener: (reset: { gap: boolean }) => void) {
      resetListeners.add(listener);
      return () => resetListeners.delete(listener);
    },
    close() {
      closed.set(true);
    }
  };
  return {
    chatto: connection as unknown as ChattoConnection,
    emit(event: RealtimeEvent) {
      for (const listener of [...eventListeners]) listener(event);
    },
    reset(gap = false) {
      for (const listener of [...resetListeners]) listener({ gap });
    },
    setStatus(value: ChattoConnectionStatus) {
      status.set(value);
    },
    endSession() {
      sessionEnded.set(true);
    },
    get listenerCount() {
      return eventListeners.size + resetListeners.size;
    }
  };
}
