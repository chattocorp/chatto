import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createEventBusHandlerRegistrar, getRealtimeEventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind } from '$lib/render/eventKinds';
import {
  RealtimeEventEnvelope,
  RealtimeClose,
  RealtimeError,
  RealtimeHeartbeat,
  RealtimeMentionNotificationEvent,
  RealtimeServerFrame,
  RealtimeServerHello,
  RealtimeServerUpdatedEvent,
  RealtimeSubscribed
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { eventBusManager, setRealtimeSocketFactoryForTests } from './eventBus.svelte';
import type { ConnectionStatus, ServerConnection } from './serverConnection.svelte';

class FakeRealtimeSocket {
  binaryType: BinaryType = 'blob';
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: Uint8Array | ArrayBuffer | Blob }) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: { code?: number; reason?: string }) => void) | null = null;
  sent: Uint8Array[] = [];
  closeCalls: Array<{ code?: number; reason?: string }> = [];

  constructor(readonly url: string) {}

  send(data: Uint8Array): void {
    this.sent.push(data);
  }

  close(code?: number, reason?: string): void {
    this.readyState = 3;
    this.closeCalls.push({ code, reason });
    this.onclose?.({ code, reason });
  }

  open(): void {
    this.readyState = 1;
    this.onopen?.();
  }

  async receive(frame: RealtimeServerFrame): Promise<void> {
    this.onmessage?.({ data: frame.toBinary() });
    await Promise.resolve();
  }

  serverClose(code = 1006, reason = 'closed'): void {
    this.readyState = 3;
    this.onclose?.({ code, reason });
  }
}

class FakeServerConnection {
  status: ConnectionStatus = $state('connecting');
  reconnectCount = $state(0);
  realtimeUrl = 'ws://chatto.test/api/realtime';
  bearerToken: string | null = 'token-1';
  client = {};
  statusUpdates: ConnectionStatus[] = [];
  authRequiredCalls = 0;
  #reconnect: ((reason: string) => void) | null = null;
  #wasDisconnected = false;

  setRealtimeConnectionStatus(status: ConnectionStatus): void {
    if (status === 'disconnected') {
      if (this.status === 'connected') this.#wasDisconnected = true;
      this.status = status;
      this.statusUpdates.push(status);
      return;
    }
    if (status === 'connected' && this.#wasDisconnected) {
      this.#wasDisconnected = false;
      this.reconnectCount++;
    }
    this.status = status;
    this.statusUpdates.push(status);
  }

  registerRealtimeReconnect(handler: (reason: string) => void): () => void {
    this.#reconnect = handler;
    return () => {
      if (this.#reconnect === handler) this.#reconnect = null;
    };
  }

  forceReconnect(reason: string): void {
    this.#reconnect?.(reason);
  }

  handleAuthenticationRequired(): void {
    this.authRequiredCalls++;
  }
}

const TEST_SERVER = 'test-server-bus';
let sockets: FakeRealtimeSocket[];

function serverFrame(frame: RealtimeServerFrame['frame']): RealtimeServerFrame {
  return new RealtimeServerFrame({ frame });
}

function helloFrame(): RealtimeServerFrame {
  return serverFrame({
    case: 'hello',
    value: new RealtimeServerHello({
      protocolVersion: 1,
      serverVersion: 'test',
      heartbeatIntervalSeconds: 10
    })
  });
}

function subscribedFrame(): RealtimeServerFrame {
  return serverFrame({ case: 'subscribed', value: new RealtimeSubscribed() });
}

function serverUpdatedFrame(id = 'evt-1'): RealtimeServerFrame {
  return serverFrame({
    case: 'event',
    value: new RealtimeEventEnvelope({
      id,
      createdAt: Timestamp.now(),
      event: {
        case: 'serverUpdated',
        value: new RealtimeServerUpdatedEvent({
          name: 'Updated',
          description: 'Description',
          logoUrl: 'https://example.test/logo.png'
        })
      }
    })
  });
}

function heartbeatFrame(): RealtimeServerFrame {
  return serverFrame({
    case: 'heartbeat',
    value: new RealtimeHeartbeat({ id: 'heartbeat-1', createdAt: Timestamp.now() })
  });
}

function mentionNotificationFrame(): RealtimeServerFrame {
  return serverFrame({
    case: 'event',
    value: new RealtimeEventEnvelope({
      id: 'evt-mention',
      createdAt: Timestamp.now(),
      actorId: 'user-1',
      event: {
        case: 'mentionNotification',
        value: new RealtimeMentionNotificationEvent({
          roomId: 'room-1',
          actorUserId: 'user-1',
          actorDisplayName: 'Ada Lovelace',
          roomName: 'General'
        })
      }
    })
  });
}

async function startAndSubscribe(fake = new FakeServerConnection()): Promise<{
  fake: FakeServerConnection;
  socket: FakeRealtimeSocket;
}> {
  eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
  const socket = sockets.at(-1);
  if (!socket) throw new Error('expected realtime socket');
  socket.open();
  await socket.receive(helloFrame());
  await socket.receive(subscribedFrame());
  return { fake, socket };
}

describe('eventBusManager realtime transport', () => {
  let consoleError: ReturnType<typeof vi.spyOn>;
  let consoleWarn: ReturnType<typeof vi.spyOn>;
  let consoleDebug: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    sockets = [];
    setRealtimeSocketFactoryForTests((url) => {
      const socket = new FakeRealtimeSocket(url);
      sockets.push(socket);
      return socket;
    });
    consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    consoleDebug = vi.spyOn(console, 'debug').mockImplementation(() => {});
  });

  afterEach(() => {
    eventBusManager.resumeAll();
    eventBusManager.stopBus(TEST_SERVER);
    setRealtimeSocketFactoryForTests(null);
    consoleError.mockRestore();
    consoleWarn.mockRestore();
    consoleDebug.mockRestore();
    vi.useRealTimers();
  });

  it('opens /api/realtime, sends hello, then subscribes after server hello', async () => {
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].url).toBe(fake.realtimeUrl);
    sockets[0].open();
    expect(sockets[0].sent).toHaveLength(1);

    await sockets[0].receive(helloFrame());
    expect(sockets[0].sent).toHaveLength(2);
    await sockets[0].receive(subscribedFrame());
    expect(fake.status).toBe('connected');
  });

  it('dispatches protobuf realtime events to existing event handlers', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    await socket.receive(serverUpdatedFrame());

    expect(handler).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'evt-1',
        event: expect.objectContaining({
          kind: RoomEventKind.ServerUpdated,
          name: 'Updated'
        })
      })
    );
    expect(consoleDebug).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] event dispatched`,
      RoomEventKind.ServerUpdated,
      expect.objectContaining({ eventId: 'evt-1' })
    );
  });

  it('attaches the decoded protobuf event to dispatched envelopes', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    await socket.receive(mentionNotificationFrame());

    const dispatched = handler.mock.calls[0]?.[0];
    expect(dispatched).toEqual(
      expect.objectContaining({
        event: expect.objectContaining({
          kind: RoomEventKind.MentionNotification
        })
      })
    );
    const realtime = getRealtimeEventEnvelope(dispatched);
    expect(realtime?.event.case).toBe('mentionNotification');
    expect(realtime?.event.value).toEqual(
      expect.objectContaining({
        actorDisplayName: 'Ada Lovelace',
        roomName: 'General'
      })
    );
  });

  it('isolates handler errors so one throwing handler does not stop the others', async () => {
    const { socket } = await startAndSubscribe();
    const ranBefore = vi.fn();
    const ranAfter = vi.fn();
    const bus = eventBusManager.getBus(TEST_SERVER)!;
    bus.handlers.add(ranBefore);
    bus.handlers.add(() => {
      throw new Error('handler boom');
    });
    bus.handlers.add(ranAfter);

    await socket.receive(serverUpdatedFrame());

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
  });

  it('continues delivering events after a handler error on a previous event', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    let throwOnce = true;
    const bus = eventBusManager.getBus(TEST_SERVER)!;
    bus.handlers.add(() => {
      if (throwOnce) {
        throwOnce = false;
        throw new Error('handler boom');
      }
    });
    bus.handlers.add(handler);

    await socket.receive(serverUpdatedFrame('evt-1'));
    await socket.receive(serverUpdatedFrame('evt-2'));

    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('reconnects and notifies catch-up handlers when the socket closes', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    socket.serverClose();

    expect(fake.status).toBe('disconnected');
    expect(catchUp).toHaveBeenCalledWith({
      reason: 'subscription-ended',
      phase: 'immediate'
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('does not reconnect when the realtime stream reports authentication required', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    await socket.receive(
      serverFrame({
        case: 'error',
        value: new RealtimeError({
          code: 'authentication_required',
          message: 'session expired',
          fatal: true
        })
      })
    );

    expect(fake.authRequiredCalls).toBe(1);
    expect(fake.status).toBe('disconnected');
    expect(catchUp).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(1);
  });

  it('does not reconnect when the realtime stream closes for authentication required', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: 'authentication_required',
          message: 'session expired',
          reconnect: true
        })
      })
    );

    expect(fake.authRequiredCalls).toBe(1);
    expect(fake.status).toBe('disconnected');
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(1);
  });

  it('re-notifies catch-up handlers after the projection grace period', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe();
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    socket.serverClose();

    expect(catchUp).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2_499);
    expect(catchUp).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(catchUp).toHaveBeenCalledTimes(2);
    expect(catchUp).toHaveBeenNthCalledWith(2, {
      reason: 'subscription-ended',
      phase: 'projection-grace'
    });
  });

  it('reconnects when the ServerConnection retry bridge requests it', async () => {
    vi.useFakeTimers();
    const { fake } = await startAndSubscribe();
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    fake.forceReconnect('user retry');

    expect(catchUp).toHaveBeenCalledWith({
      reason: 'ws-reconnected',
      phase: 'immediate'
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('reconnects and notifies catch-up handlers when heartbeats stall', async () => {
    vi.useFakeTimers();
    await startAndSubscribe();
    const catchUp = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.catchUpHandlers.add(catchUp);

    await vi.advanceTimersByTimeAsync(90_000);

    expect(catchUp).toHaveBeenCalledWith({
      reason: 'heartbeat-stalled',
      phase: 'immediate'
    });
    expect(sockets).toHaveLength(2);
  });

  it('does not dispatch heartbeat frames to handlers', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    await socket.receive(heartbeatFrame());

    expect(handler).not.toHaveBeenCalled();
  });

  it('treats room universal changes as room layout updates', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    const unsubscribe = createEventBusHandlerRegistrar(TEST_SERVER)!.onRoomLayoutUpdated(handler);

    await socket.receive(
      serverFrame({
        case: 'event',
        value: new RealtimeEventEnvelope({
          id: 'evt-room',
          createdAt: Timestamp.now(),
          event: {
            case: 'roomUniversalChanged',
            value: { roomId: 'room-1', universal: false }
          }
        })
      })
    );

    expect(handler).toHaveBeenCalledWith({ roomId: 'room-1', universal: false });
    unsubscribe();
  });

  it('matches direct room layout handlers by local event kind', async () => {
    await startAndSubscribe();
    const handler = vi.fn();
    const unsubscribe = createEventBusHandlerRegistrar(TEST_SERVER)!.onRoomLayoutUpdated(handler);
    const bus = eventBusManager.getBus(TEST_SERVER)!;

    for (const eventHandler of bus.handlers) {
      eventHandler({
        id: 'evt-room-kind',
        createdAt: new Date().toISOString(),
        actorId: null,
        actor: null,
        event: {
          kind: RoomEventKind.RoomUniversalChanged,
          roomId: 'room-kind',
          universal: true
        } as never
      });
    }

    expect(handler).toHaveBeenCalledWith({ roomId: 'room-kind', universal: true });
    unsubscribe();
  });

  it('does NOT reconnect when stopBus is called', async () => {
    await startAndSubscribe();
    expect(sockets).toHaveLength(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].closeCalls).toHaveLength(1);
  });

  it('pauseAll stops active buses and blocks later startBus calls until resumeAll', async () => {
    const fake = new FakeServerConnection();
    await startAndSubscribe(fake);
    expect(sockets).toHaveLength(1);

    eventBusManager.pauseAll();
    expect(eventBusManager.getBus(TEST_SERVER)).toBeUndefined();

    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
    expect(sockets).toHaveLength(1);
    expect(eventBusManager.getBus(TEST_SERVER)).toBeUndefined();

    eventBusManager.resumeAll();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
    expect(sockets).toHaveLength(2);
    expect(eventBusManager.getBus(TEST_SERVER)).toBeDefined();
  });
});
