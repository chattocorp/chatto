import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { makeSubject, type Source, type Subject } from 'wonka';
import type { Client } from '@urql/svelte';
import {
  eventBusManager,
  setRealtimeSocketFactoryForTests
} from '$lib/state/server/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import Harness from './UseMayHaveMissedMessagesCallbackHarness.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeServerId: 'test-server'
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({ reconnectCount: 0 })
}));

class FakeGqlClient {
  reconnectCount = $state(0);
  realtimeUrl = 'ws://test-server/api/realtime';
  bearerToken: string | null = null;
  setRealtimeConnectionStatus = vi.fn();
  registerRealtimeReconnect = vi.fn(() => () => {});
  handleAuthenticationRequired = vi.fn();
  #subjects: Subject<{ data?: unknown; error?: unknown }>[] = [];
  client: Client;

  constructor() {
    this.client = {
      subscription: vi.fn().mockImplementation(() => {
        const subj = makeSubject<{ data?: unknown; error?: unknown }>();
        this.#subjects.push(subj);
        return subj.source as unknown as Source<unknown>;
      }),
      query: vi.fn(),
      mutation: vi.fn()
    } as unknown as Client;
  }
}

const TEST_SERVER = 'test-server';

describe('useMayHaveMissedMessagesCallback', () => {
  let consoleDebug: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    mocks.activeServerId = TEST_SERVER;
    setRealtimeSocketFactoryForTests(() => ({
      binaryType: 'arraybuffer',
      readyState: 0,
      onopen: null,
      onmessage: null,
      onerror: null,
      onclose: null,
      send: vi.fn(),
      close: vi.fn()
    }));
    consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
    eventBusManager.stopBus(TEST_SERVER);
    setRealtimeSocketFactoryForTests(null);
    consoleDebug.mockRestore();
    vi.restoreAllMocks();
  });

  it('runs the callback when the active event bus reports a catch-up gap', async () => {
    const fake = new FakeGqlClient();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
    const onSignal = vi.fn();

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    const bus = eventBusManager.getBus(TEST_SERVER);
    if (!bus) throw new Error('event bus did not start');
    await vi.waitFor(() => expect(bus.catchUpHandlers.size).toBe(1));

    for (const handler of bus.catchUpHandlers) {
      handler('heartbeat-stalled');
    }

    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledWith('event-bus-heartbeat-stalled'));
    rendered.unmount();
  });

  it('does not let a failed wake refresh suppress a queued online signal', async () => {
    let resolveFirst!: (value: boolean) => void;
    const firstRefresh = new Promise<boolean>((resolve) => {
      resolveFirst = resolve;
    });
    const onSignal = vi
      .fn()
      .mockImplementationOnce(() => firstRefresh)
      .mockResolvedValue(undefined);

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    window.dispatchEvent(new Event('pageshow'));
    window.dispatchEvent(new Event('online'));
    expect(onSignal).toHaveBeenCalledTimes(1);
    expect(onSignal).toHaveBeenNthCalledWith(1, 'pageshow');

    resolveFirst(false);

    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledTimes(2));
    expect(onSignal).toHaveBeenNthCalledWith(2, 'online');
    rendered.unmount();
  });

  it('runs a queued event-bus catch-up even after the in-flight refresh succeeds', async () => {
    const fake = new FakeGqlClient();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as GraphQLClient);
    let resolveFirst!: (value: boolean) => void;
    const firstRefresh = new Promise<boolean>((resolve) => {
      resolveFirst = resolve;
    });
    const onSignal = vi
      .fn()
      .mockImplementationOnce(() => firstRefresh)
      .mockResolvedValue(undefined);

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    const bus = eventBusManager.getBus(TEST_SERVER);
    if (!bus) throw new Error('event bus did not start');
    await vi.waitFor(() => expect(bus.catchUpHandlers.size).toBe(1));

    for (const handler of bus.catchUpHandlers) {
      handler('subscription-ended');
      handler('heartbeat-stalled');
    }
    expect(onSignal).toHaveBeenCalledTimes(1);
    expect(onSignal).toHaveBeenNthCalledWith(1, 'event-bus-subscription-ended');

    resolveFirst(true);

    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledTimes(2));
    expect(onSignal).toHaveBeenNthCalledWith(2, 'event-bus-heartbeat-stalled');
    rendered.unmount();
  });
});
