import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Event as DurableEvent } from '$lib/pb/chatto/core/v1/event_pb';
import { HeartbeatEvent } from '$lib/pb/chatto/core/v1/live_events_pb';
import { MessagePostedEvent } from '$lib/pb/chatto/core/v1/message_events_pb';
import { StreamEvent } from '$lib/pb/chatto/wire/v1/protocol_pb';
import type { ServerConnection } from './serverConnection.svelte';

const { mockStartWireEventBus, mockStopWireEventBus, wireBuses } = vi.hoisted(() => {
  const buses = new Map<string, { handlers: Set<(event: StreamEvent) => void> }>();

  return {
    wireBuses: buses,
    mockStartWireEventBus: vi.fn((serverId: string) => {
      if (!buses.has(serverId)) {
        buses.set(serverId, { handlers: new Set<(event: StreamEvent) => void>() });
      }
      return () => mockStopWireEventBus(serverId);
    }),
    mockStopWireEventBus: vi.fn((serverId: string) => {
      buses.delete(serverId);
    })
  };
});

vi.mock('./wireEventBus.svelte', () => ({
  wireEventBusManager: {
    startBus: mockStartWireEventBus,
    stopBus: mockStopWireEventBus,
    getBus: (serverId: string) => wireBuses.get(serverId),
    getClient: vi.fn()
  }
}));

import { eventBusManager } from './eventBus.svelte';

const TEST_SERVER = 'test-server-bus';

function fakeServerConnection(): ServerConnection {
  return {
    wireUrl: 'ws://example.test/api/wire',
    token: 'test-token'
  } as unknown as ServerConnection;
}

function dispatchWireEvent(event: StreamEvent): void {
  const bus = wireBuses.get(TEST_SERVER);
  if (!bus) throw new Error('wire bus did not start');
  for (const handler of bus.handlers) handler(event);
}

function messagePostedStreamEvent(id = 'evt_1', roomId = 'room_1'): StreamEvent {
  return new StreamEvent({
    eventId: id,
    eventType: 'message_posted',
    payload: {
      case: 'durableEvent',
      value: new DurableEvent({
        id,
        actorId: 'user_1',
        event: {
          case: 'messagePosted',
          value: new MessagePostedEvent({ roomId })
        }
      })
    }
  });
}

function heartbeatStreamEvent(): StreamEvent {
  return new StreamEvent({
    eventId: 'heartbeat',
    eventType: 'heartbeat',
    payload: {
      case: 'heartbeat',
      value: new HeartbeatEvent()
    }
  });
}

describe('eventBusManager wire bridge', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;
  let consoleWarn: ReturnType<typeof vi.spyOn>;
  let consoleDebug: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    eventBusManager.stopAll();
    wireBuses.clear();
    mockStartWireEventBus.mockClear();
    mockStopWireEventBus.mockClear();
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  });

  afterEach(() => {
    eventBusManager.stopAll();
    wireBuses.clear();
    consoleError.mockRestore();
    consoleWarn.mockRestore();
    consoleDebug.mockRestore();
    vi.useRealTimers();
  });

  it('starts and stops the wire event bus with the public bus', () => {
    const client = fakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, client);

    expect(mockStartWireEventBus).toHaveBeenCalledWith(TEST_SERVER, client);
    expect(wireBuses.get(TEST_SERVER)?.handlers.size).toBe(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(mockStopWireEventBus).toHaveBeenCalledWith(TEST_SERVER);
    expect(eventBusManager.getBus(TEST_SERVER)).toBeUndefined();
    expect(wireBuses.get(TEST_SERVER)).toBeUndefined();
  });

  it('does not start a duplicate wire event bus for an existing public bus', () => {
    const client = fakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, client);
    eventBusManager.startBus(TEST_SERVER, client);

    expect(mockStartWireEventBus).toHaveBeenCalledTimes(1);
    expect(wireBuses.get(TEST_SERVER)?.handlers.size).toBe(1);
  });

  it('dispatches decoded wire events to public handlers', () => {
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    dispatchWireEvent(messagePostedStreamEvent('evt_message', 'room_1'));

    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler.mock.calls[0][0]).toMatchObject({
      id: 'evt_message',
      actorId: 'user_1',
      event: {
        __typename: 'MessagePostedEvent',
        roomId: 'room_1'
      }
    });
  });

  it('isolates handler errors so one throwing handler does not stop the others', () => {
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const bus = eventBusManager.getBus(TEST_SERVER)!;
    const ranBefore = vi.fn();
    const ranAfter = vi.fn();

    bus.handlers.add(ranBefore);
    bus.handlers.add(() => {
      throw new Error('handler boom');
    });
    bus.handlers.add(ranAfter);

    dispatchWireEvent(messagePostedStreamEvent());

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError).toHaveBeenCalled();
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
  });

  it('continues delivering events after a handler error on a previous event', () => {
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const bus = eventBusManager.getBus(TEST_SERVER)!;
    const handler = vi.fn();
    let throwOnce = true;

    bus.handlers.add(() => {
      if (!throwOnce) return;
      throwOnce = false;
      throw new Error('handler boom');
    });
    bus.handlers.add(handler);

    dispatchWireEvent(messagePostedStreamEvent('evt_1'));
    dispatchWireEvent(messagePostedStreamEvent('evt_2'));

    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('does not dispatch heartbeat events to public handlers', () => {
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    dispatchWireEvent(heartbeatStreamEvent());

    expect(handler).not.toHaveBeenCalled();
  });

  it('notifies catch-up handlers when heartbeats stall', async () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    await vi.advanceTimersByTimeAsync(75_000);

    expect(catchUp).toHaveBeenCalledTimes(1);
    expect(catchUp).toHaveBeenNthCalledWith(1, 'heartbeat-stalled');
    expect(
      consoleWarn.mock.calls.some((call: unknown[]) => String(call[0]).includes('heartbeat stalled'))
    ).toBe(true);
  });

  it('re-notifies catch-up handlers after the projection grace period', async () => {
    vi.useFakeTimers();
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    await vi.advanceTimersByTimeAsync(75_000);
    expect(catchUp).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(2_499);
    expect(catchUp).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(catchUp).toHaveBeenCalledTimes(2);
    expect(catchUp).toHaveBeenNthCalledWith(2, 'heartbeat-stalled');
  });

  it('removes the wire bridge handler before stopping the wire bus', () => {
    eventBusManager.startBus(TEST_SERVER, fakeServerConnection());
    const wireBus = wireBuses.get(TEST_SERVER);
    if (!wireBus) throw new Error('wire bus did not start');

    expect(wireBus.handlers.size).toBe(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(wireBus.handlers.size).toBe(0);
    expect(mockStopWireEventBus).toHaveBeenCalledWith(TEST_SERVER);
  });
});
