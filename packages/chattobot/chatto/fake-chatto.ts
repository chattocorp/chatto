/**
 * Test double for `connectChatto` and `createChattoApi`. Each connection
 * records its credentials and answers service calls from an in-memory Connect
 * router, like a real server. Like a real connection, a closed connection
 * fails its requests.
 */

import { vi } from 'vitest';
import type { ServiceType } from '@bufbuild/protobuf';
import {
  Code,
  ConnectError,
  createClient,
  createRouterTransport,
  type ConnectRouter
} from '@connectrpc/connect';
import type { ChattoApi, ChattoApiOptions } from '@chatto/client/apiClient';
import type {
  ChattoConnection,
  ChattoConnectionStatus,
  ConnectChattoOptions,
  RealtimeEvent
} from '@chatto/client';
import { signal } from '@chatto/client/reactivity';

export interface FakeConnection {
  options: ConnectChattoOptions;
  closed: boolean;
  /** Service methods that this connection called, as `Service/Method`. */
  calls: string[];
  emit(event: RealtimeEvent): void;
  readonly listening: boolean;
}

export interface FakeChattoSetup {
  viewerId: string | (() => Promise<string>);
  routes: (router: ConnectRouter) => void;
}

/** Create the connection factory and the list of created connections. */
export function fakeChatto(setup: FakeChattoSetup) {
  const connections: FakeConnection[] = [];
  const connectChatto = vi.fn((options: ConnectChattoOptions): ChattoConnection => {
    const eventListeners = new Set<(event: RealtimeEvent) => void>();
    const status = signal<ChattoConnectionStatus>('connected');
    const fake: FakeConnection = {
      options,
      closed: false,
      calls: [],
      emit(event) {
        for (const listener of [...eventListeners]) listener(event);
      },
      get listening() {
        return eventListeners.size > 0;
      }
    };
    connections.push(fake);
    const transport = createRouterTransport(setup.routes, {
      router: {
        interceptors: [
          (next) => async (request) => {
            if (fake.closed) throw new ConnectError('connection closed', Code.Canceled);
            fake.calls.push(`${request.service.typeName.split('.').pop()}/${request.method.name}`);
            return next(request);
          }
        ]
      }
    });
    const connection = {
      serverId: 'test',
      connection: {
        get status() {
          return status.get();
        }
      },
      get status() {
        return fake.closed ? 'disconnected' : status.get();
      },
      sessionEnded: false,
      get closed() {
        return fake.closed;
      },
      async ready({ signal }: { signal?: AbortSignal } = {}) {
        signal?.throwIfAborted();
        const viewerId =
          typeof setup.viewerId === 'string' ? setup.viewerId : await setup.viewerId();
        return { viewerId };
      },
      service<T extends ServiceType>(service: T) {
        return createClient(service, transport);
      },
      onEvent(listener: (event: RealtimeEvent) => void) {
        eventListeners.add(listener);
        return () => eventListeners.delete(listener);
      },
      onReset() {
        return () => {};
      },
      close() {
        fake.closed = true;
      }
    };
    return connection as unknown as ChattoConnection;
  });
  /** Stateless API clients, with their credentials and the methods they called. */
  const apis: { options: ChattoApiOptions; calls: string[] }[] = [];
  const createChattoApi = vi.fn((options: ChattoApiOptions): ChattoApi => {
    const api = { options, calls: [] as string[] };
    apis.push(api);
    const transport = createRouterTransport(setup.routes, {
      router: {
        interceptors: [
          (next) => async (request) => {
            api.calls.push(`${request.service.typeName.split('.').pop()}/${request.method.name}`);
            return next(request);
          }
        ]
      }
    });
    return {
      service: <T extends ServiceType>(service: T) => createClient(service, transport)
    } as unknown as ChattoApi;
  });
  return { connectChatto, connections, createChattoApi, apis };
}

/** Let queued event handlers and in-memory requests finish. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 20; i++) await new Promise((resolve) => setTimeout(resolve, 0));
}
