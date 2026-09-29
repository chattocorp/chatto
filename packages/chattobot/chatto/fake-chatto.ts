/**
 * Test double for `createClient` and `createApi`. Each connection records its
 * credentials and answers service calls from an in-memory Connect router, like
 * a real server. Like a real connection, a closed connection fails its
 * requests. Event consumption is simplified: events are handled in order,
 * and the stream reports `ready` without gaps; the client tests cover the
 * rest.
 */

import { vi } from 'vitest';
import type { ServiceType } from '@bufbuild/protobuf';
import {
  Code,
  ConnectError,
  createClient as createServiceClient,
  createRouterTransport,
  type ConnectRouter
} from '@connectrpc/connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import {
  Api,
  MessagingRequests,
  type ApiOptions,
  type ChattoClient,
  type ConnectOptions,
  type Server,
  type ConsumeEventsOptions,
  type RealtimeEvent
} from '@chatto/client';

export interface FakeConnection {
  options: ConnectOptions;
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

/** Create the client and API factories and the lists of what they created. */
export function fakeChatto(setup: FakeChattoSetup) {
  const viewerId = async () =>
    typeof setup.viewerId === 'string' ? setup.viewerId : await setup.viewerId();
  /** The routes, plus a viewer read that answers with the setup's viewer. */
  const routes = (router: ConnectRouter) => {
    setup.routes(router);
    router.service(ViewerService, {
      getViewer: async () => ({ user: { profile: { id: await viewerId() } } })
    });
  };

  const connections: FakeConnection[] = [];
  function connect(options: ConnectOptions): Server {
    const queue: RealtimeEvent[] = [];
    let wake: (() => void) | undefined;
    let consumers = 0;
    const fake: FakeConnection = {
      options,
      closed: false,
      calls: [],
      emit(event) {
        queue.push(event);
        wake?.();
      },
      get listening() {
        return consumers > 0;
      }
    };
    connections.push(fake);
    const transport = createRouterTransport(routes, {
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
    const service = <T extends ServiceType>(type: T) => createServiceClient(type, transport);
    const requests = new MessagingRequests({ service }, viewerId);
    const connection = {
      serverId: 'test',
      get closed() {
        return fake.closed;
      },
      async ready({ signal }: { signal?: AbortSignal } = {}) {
        signal?.throwIfAborted();
        return { viewerId: await viewerId() };
      },
      service,
      addressedMessage: requests.addressedMessage.bind(requests),
      async consumeEvents({ signal, onEvent, onStatus }: ConsumeEventsOptions) {
        consumers++;
        const loop = new AbortController();
        const stop = () => wake?.();
        signal?.addEventListener('abort', stop, { once: true });
        try {
          onStatus?.({ state: 'ready', gap: false });
          while (!signal?.aborted && !fake.closed) {
            const event = queue.shift();
            if (!event) {
              await new Promise<void>((resolve) => (wake = resolve));
              wake = undefined;
              continue;
            }
            await onEvent(event, { signal: loop.signal });
          }
        } finally {
          loop.abort();
          consumers--;
          signal?.removeEventListener('abort', stop);
        }
      },
      close() {
        fake.closed = true;
        wake?.();
      }
    };
    return connection as unknown as Server;
  }

  const createClient = vi.fn((): ChattoClient => {
    const opened: Server[] = [];
    return {
      connect(options: ConnectOptions) {
        const connection = connect(options);
        opened.push(connection);
        return connection;
      },
      close() {
        for (const connection of opened) connection.close();
      }
    } as unknown as ChattoClient;
  });

  /** Stateless API clients, with their credentials and the methods they called. */
  const apis: { options: ApiOptions; calls: string[] }[] = [];
  const createApi = vi.fn((options: ApiOptions): Api => {
    const api = { options, calls: [] as string[] };
    apis.push(api);
    const transport = createRouterTransport(routes, {
      router: {
        interceptors: [
          (next) => async (request) => {
            api.calls.push(`${request.service.typeName.split('.').pop()}/${request.method.name}`);
            return next(request);
          }
        ]
      }
    });
    return new Api(
      options.serverUrl,
      (type) => createServiceClient(type, transport),
      options.viewerId
    );
  });
  return { createClient, connections, createApi, apis };
}

/** Let queued event handlers and in-memory requests finish. */
export async function settle(): Promise<void> {
  for (let i = 0; i < 20; i++) await new Promise((resolve) => setTimeout(resolve, 0));
}
