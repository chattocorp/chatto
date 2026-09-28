import type { ServiceType } from '@bufbuild/protobuf';
import {
  createRouterTransport,
  type ConnectRouter,
  type HandlerContext,
  type ServiceImpl
} from '@connectrpc/connect';
import { vi, type Mock } from 'vitest';
import type { ConnectAPIConfig } from '$lib/api-client/connect';

/** Service implementations of a fake Chatto server, registered with `router.service(...)`. */
export type FakeServerRoutes = (router: ConnectRouter) => void;

/**
 * An API config whose requests go to an in-memory fake server instead of HTTP.
 *
 * The real generated client, Chatto's interceptors, and protobuf serialization
 * all run. Handlers receive typed request messages and can return plain objects
 * or throw `ConnectError`s:
 *
 * ```ts
 * const api = createBotAPI(
 *   fakeServer((router) => router.service(BotService, { listBots: () => ({ bots: [] }) }))
 * );
 * ```
 */
export function fakeServer(
  routes: FakeServerRoutes,
  config: Partial<ConnectAPIConfig> = {}
): ConnectAPIConfig {
  return {
    baseUrl: 'https://chatto.test',
    bearerToken: null,
    ...config,
    transport: (interceptors) => createRouterTransport(routes, { transport: { interceptors } })
  };
}

/** Mock handlers for every method of a service, typed with the method's signature. */
export type MockService<T extends ServiceType> = {
  [K in keyof ServiceImpl<T>]: Mock<ServiceImpl<T>[K]>;
};

/**
 * Create a `vi.fn` handler for every method of a service. Register the result
 * with `router.service(Service, handlers)`. The handlers keep the method types,
 * so a fixture with the wrong response shape fails type checking.
 */
export function mockService<T extends ServiceType>(service: T): MockService<T> {
  return Object.fromEntries(
    Object.keys(service.methods).map((name) => [name, vi.fn()])
  ) as MockService<T>;
}

type Handler = (...args: never[]) => unknown;

/** The request message that a mock handler received in one call. */
export function receivedRequest<T extends Handler>(
  handler: Mock<T>,
  call = 0
): Parameters<T>[0] | undefined {
  return handler.mock.calls[call]?.[0];
}

/** The call context (headers, timeout, signal) that a mock handler received in one call. */
export function receivedContext(
  handler: { mock: { calls: readonly (readonly unknown[])[] } },
  call = 0
): HandlerContext | undefined {
  return handler.mock.calls[call]?.[1] as HandlerContext | undefined;
}
