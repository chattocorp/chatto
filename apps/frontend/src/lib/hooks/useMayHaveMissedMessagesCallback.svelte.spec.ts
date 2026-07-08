import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import {
  eventBusManager,
  setRealtimeSocketFactoryForTests
} from '$lib/state/server/eventBus.svelte';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { emitServerResumeSignal } from './resumeCoordinator.svelte';
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

class FakeServerConnection {
  reconnectCount = $state(0);
  realtimeUrl = 'ws://test-server/api/realtime';
  bearerToken: string | null = null;
  setRealtimeConnectionStatus = vi.fn();
  registerRealtimeReconnect = vi.fn(() => () => {});
  handleAuthenticationRequired = vi.fn();
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
    vi.useFakeTimers();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
    const onSignal = vi.fn();

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    const bus = eventBusManager.getBus(TEST_SERVER);
    if (!bus) throw new Error('event bus did not start');
    await vi.waitFor(() => expect(bus.catchUpHandlers.size).toBe(1));

    for (const handler of bus.catchUpHandlers) {
      handler({ reason: 'heartbeat-stalled', phase: 'immediate' });
    }
    await vi.advanceTimersByTimeAsync(1_000);

    expect(onSignal).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: TEST_SERVER,
        reason: 'event-bus-heartbeat-stalled',
        phase: 'immediate',
        source: 'event-bus'
      })
    );
    rendered.unmount();
  });

  it('coalesces a browser wake burst into one signal', async () => {
    vi.useFakeTimers();
    const onSignal = vi.fn().mockResolvedValue(undefined);

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    window.dispatchEvent(new Event('pageshow'));
    window.dispatchEvent(new Event('online'));
    expect(onSignal).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1_000);
    expect(onSignal).toHaveBeenCalledOnce();
    expect(onSignal).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: TEST_SERVER,
        reason: 'online',
        phase: 'immediate',
        source: 'browser'
      })
    );
    rendered.unmount();
  });

  it('does not dedupe event-bus catch-up after a skipped refresh', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-08T12:00:00Z'));
    const onSignal = vi.fn().mockResolvedValueOnce(false).mockResolvedValue(undefined);

    const rendered = render(Harness, { props: { onSignal } });
    flushSync();

    emitServerResumeSignal(
      TEST_SERVER,
      {
        reason: 'visibility',
        source: 'browser',
        hiddenDurationMs: 1_000
      },
      { coalesceMs: 0 }
    );
    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledTimes(1));

    emitServerResumeSignal(
      TEST_SERVER,
      {
        reason: 'event-bus-ws-reconnected',
        source: 'event-bus'
      },
      { coalesceMs: 0 }
    );

    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledTimes(2));
    expect(onSignal).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        reason: 'event-bus-ws-reconnected',
        source: 'event-bus'
      })
    );
    rendered.unmount();
  });

  it('runs a projection-grace event-bus catch-up after the in-flight refresh succeeds', async () => {
    vi.useFakeTimers();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
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
      handler({ reason: 'subscription-ended', phase: 'immediate' });
    }
    await vi.advanceTimersByTimeAsync(1_000);

    expect(onSignal).toHaveBeenCalledTimes(1);
    expect(onSignal).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({
        reason: 'event-bus-subscription-ended',
        phase: 'immediate',
        source: 'event-bus'
      })
    );

    for (const handler of bus.catchUpHandlers) {
      handler({ reason: 'subscription-ended', phase: 'projection-grace' });
    }
    await vi.advanceTimersByTimeAsync(1_000);

    resolveFirst(true);

    await vi.waitFor(() => expect(onSignal).toHaveBeenCalledTimes(2));
    expect(onSignal).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        reason: 'event-bus-subscription-ended',
        phase: 'projection-grace',
        source: 'event-bus'
      })
    );
    rendered.unmount();
  });
});
