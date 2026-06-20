import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { eventBusManager } from './eventBus.svelte';
import { serverRegistry } from './registry.svelte';
import { startClientLiveSubscription } from './clientLive';
import type { GraphQLClient } from './graphqlClient.svelte';
import type { EventEnvelope } from '$lib/eventBus.svelte';

const TEST_SERVER = 'test-server-bus';

type LiveSubscriptionOptions = Parameters<typeof startClientLiveSubscription>[0];

const liveControllers = vi.hoisted(
  () =>
    [] as Array<
      LiveSubscriptionOptions & {
        unsubscribe: ReturnType<typeof vi.fn>;
        request: ReturnType<typeof vi.fn>;
      }
    >
);

vi.mock('./registry.svelte', () => ({
  serverRegistry: {
    getServer: vi.fn()
  }
}));

vi.mock('./clientLive', () => ({
  startClientLiveSubscription: vi.fn((options: LiveSubscriptionOptions) => {
    const controller = { ...options, unsubscribe: vi.fn(), request: vi.fn() };
    liveControllers.push(controller);
    return { unsubscribe: controller.unsubscribe, request: controller.request };
  })
}));

function fakeGqlClient(): GraphQLClient {
  return { reconnectCount: 0 } as unknown as GraphQLClient;
}

function mockLiveServer({
  token = 'test-token',
  live = true
}: { token?: string | null; live?: boolean } = {}) {
  vi.mocked(serverRegistry.getServer).mockReturnValue({
    id: TEST_SERVER,
    url: 'https://chat.example.test',
    name: 'Test Chatto',
    iconUrl: null,
    token,
    userId: 'U-test',
    userLogin: 'test',
    userDisplayName: 'Test',
    userAvatarUrl: null,
    live: live
      ? {
          url: 'wss://chat.example.test/api/live',
          tokenUrl: '/api/live-token',
          protocol: 'chatto.client-live-protobuf.v1'
        }
      : null,
    addedAt: Date.now()
  });
}

function eventEnvelope(event: EventEnvelope['event']): EventEnvelope {
  return {
    id: 'E-test',
    createdAt: new Date(0).toISOString(),
    actorId: 'U-test',
    actor: null,
    event
  };
}

function serverUpdatedEvent(): EventEnvelope {
  return eventEnvelope({
    __typename: 'ServerUpdatedEvent',
    name: 'Test Chatto',
    description: null,
    logoUrl: null,
    bannerUrl: null
  });
}

describe('eventBusManager client-live stream robustness', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;
  let consoleWarn: ReturnType<typeof vi.spyOn>;
  let consoleDebug: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    liveControllers.length = 0;
    vi.mocked(startClientLiveSubscription).mockClear();
    mockLiveServer();
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  });

  afterEach(() => {
    eventBusManager.stopBus(TEST_SERVER);
    consoleError.mockRestore();
    consoleWarn.mockRestore();
    consoleDebug.mockRestore();
    vi.useRealTimers();
  });

  it('logs an error when the client-live stream reports an error', () => {
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    liveControllers.at(-1)!.onError(new Error('live failed'));

    expect(consoleError).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain(TEST_SERVER);
    expect(consoleError.mock.calls[0][0]).toContain('Chatto live stream failed');
  });

  it('isolates handler errors so one throwing handler does not stop the others', () => {
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    const bus = eventBusManager.getBus(TEST_SERVER)!;
    const ranBefore = vi.fn();
    const ranAfter = vi.fn();
    bus.handlers.add(ranBefore);
    bus.handlers.add(() => {
      throw new Error('handler boom');
    });
    bus.handlers.add(ranAfter);

    liveControllers.at(-1)!.onEvent(serverUpdatedEvent());

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError).toHaveBeenCalled();
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
  });

  it('continues delivering events after a handler error on a previous event', () => {
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    const bus = eventBusManager.getBus(TEST_SERVER)!;
    const handler = vi.fn();
    let throwOnce = true;
    bus.handlers.add(() => {
      if (throwOnce) {
        throwOnce = false;
        throw new Error('handler boom');
      }
    });
    bus.handlers.add(handler);

    const event = serverUpdatedEvent();
    liveControllers.at(-1)!.onEvent(event);
    liveControllers.at(-1)!.onEvent(event);

    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('re-subscribes when the client-live stream ends', () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    liveControllers.at(-1)!.onReady?.();
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    liveControllers.at(-1)!.onEnd();

    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
    expect(eventBusManager.getBus(TEST_SERVER)!.request).toBeUndefined();
    expect(catchUp).toHaveBeenCalledWith('subscription-ended');
    expect(
      consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('stream ended'))
    ).toBe(true);

    vi.advanceTimersByTime(1_000);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);

    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);
    liveControllers.at(-1)!.onEvent(serverUpdatedEvent());
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it('exposes the active client-live request function while subscribed', () => {
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    const bus = eventBusManager.getBus(TEST_SERVER)!;
    expect(bus.request).toBe(liveControllers.at(-1)!.request);

    eventBusManager.stopBus(TEST_SERVER);
    expect(bus.request).toBeUndefined();
  });

  it('re-notifies catch-up handlers after the projection grace period', async () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    liveControllers.at(-1)!.onEnd();

    expect(catchUp).toHaveBeenCalledTimes(1);
    expect(catchUp).toHaveBeenNthCalledWith(1, 'subscription-ended');

    await vi.advanceTimersByTimeAsync(2_499);
    expect(catchUp).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(catchUp).toHaveBeenCalledTimes(2);
    expect(catchUp).toHaveBeenNthCalledWith(2, 'subscription-ended');
  });

  it('re-subscribes and notifies catch-up handlers when heartbeats stall', () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    liveControllers.at(-1)!.onReady?.();
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    vi.advanceTimersByTime(90_000);

    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);
    expect(catchUp).toHaveBeenCalledWith('heartbeat-stalled');
    expect(
      consoleWarn.mock.calls.some((c: unknown[]) => String(c[0]).includes('heartbeat stalled'))
    ).toBe(true);
  });

  it('backs off repeated client-live stream startup failures', () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);

    liveControllers.at(-1)!.onEnd();
    vi.advanceTimersByTime(999);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);

    liveControllers.at(-1)!.onEnd();
    vi.advanceTimersByTime(1_999);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(1);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(3);
  });

  it('resets reconnect backoff once the client-live stream becomes ready', () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    liveControllers.at(-1)!.onEnd();
    vi.advanceTimersByTime(1_000);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);

    liveControllers.at(-1)!.onReady?.();
    liveControllers.at(-1)!.onEnd();
    vi.advanceTimersByTime(999);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(1);
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(3);
  });

  it('does not dispatch heartbeat events to handlers', () => {
    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    liveControllers.at(-1)!.onEvent(eventEnvelope({ __typename: 'HeartbeatEvent', alive: true }));

    expect(handler).not.toHaveBeenCalled();
  });

  it('does NOT re-subscribe when stopBus is called even if the stream reports end during unsubscribe', () => {
    vi.mocked(startClientLiveSubscription).mockImplementationOnce(
      (options: LiveSubscriptionOptions) => {
        const controller = {
          ...options,
          unsubscribe: vi.fn(() => options.onEnd()),
          request: vi.fn()
        };
        liveControllers.push(controller);
        return { unsubscribe: controller.unsubscribe, request: controller.request };
      }
    );

    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());
    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
  });

  it('does not start a GraphQL fallback when live metadata is missing', () => {
    mockLiveServer({ live: false });

    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    expect(startClientLiveSubscription).not.toHaveBeenCalled();
    expect(consoleWarn.mock.calls[0][0]).toContain('does not advertise Chatto live metadata');
  });

  it('does not start a GraphQL fallback when no bearer token is available', () => {
    mockLiveServer({ token: null });

    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    expect(startClientLiveSubscription).not.toHaveBeenCalled();
    expect(consoleWarn.mock.calls[0][0]).toContain('requires an authenticated server token');
  });

  it('starts the client-live stream for same-origin cookie-auth servers without a bearer token', () => {
    const origin = window.location.origin;
    vi.mocked(serverRegistry.getServer).mockReturnValue({
      id: TEST_SERVER,
      url: origin,
      name: 'Local Chatto',
      iconUrl: null,
      token: null,
      userId: 'U-test',
      userLogin: 'test',
      userDisplayName: 'Test',
      userAvatarUrl: null,
      live: {
        url: '/api/live',
        tokenUrl: '/api/live-token',
        protocol: 'chatto.client-live-protobuf.v1'
      },
      addedAt: Date.now()
    });

    eventBusManager.startBus(TEST_SERVER, fakeGqlClient());

    expect(startClientLiveSubscription).toHaveBeenCalledTimes(1);
    expect(eventBusManager.getBus(TEST_SERVER)?.request).toBe(liveControllers.at(-1)!.request);
  });
});
