import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import { RoomEventKind } from '$lib/render/eventKinds';
import {
  RealtimeEventEnvelope,
  RealtimeClientFrame,
  RealtimeClose,
  RealtimeCaughtUp,
  RealtimeError,
  RealtimeHeartbeat,
  RealtimeMentionNotificationEvent,
  RealtimeServerFrame,
  RealtimeServerHello,
  RealtimeTypingEvent,
  RealtimeSubscribed,
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionReset,
  RealtimeProjectionRoomTimelineReplace
} from '@chatto/api-types/realtime/v1/realtime_pb';
import {
  eventBusManager,
  setRealtimePollRandomForTests,
  setRealtimeSocketFactoryForTests
} from './eventBus.svelte';
import type { ConnectionStatus, ServerConnection } from './serverConnection.svelte';
import { MAX_RETAINED_ROOM_TIMELINES, RealtimeProjectionSyncState } from './realtimeSync.svelte';

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

  async receiveBytes(data: Uint8Array): Promise<void> {
    this.onmessage?.({ data });
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

function helloFrame(heartbeatIntervalSeconds = 10): RealtimeServerFrame {
  return serverFrame({
    case: 'hello',
    value: new RealtimeServerHello({
      protocolVersion: 2,
      serverVersion: 'test',
      heartbeatIntervalSeconds
    })
  });
}

function subscribedFrame(): RealtimeServerFrame {
  return serverFrame({ case: 'subscribed', value: new RealtimeSubscribed() });
}

function projectionFrame(cursor: string | undefined): RealtimeServerFrame {
  return serverFrame({
    case: 'projectionEvent',
    value: new RealtimeProjectionEvent({
      resumeCursor: cursor,
      operations: [
        new RealtimeProjectionOperation({
          operation: { case: 'reset', value: new RealtimeProjectionReset() }
        })
      ]
    })
  });
}

function roomTimelineFrame(roomId: string): RealtimeServerFrame {
  return serverFrame({
    case: 'projectionEvent',
    value: new RealtimeProjectionEvent({
      operations: [
        new RealtimeProjectionOperation({
          operation: {
            case: 'roomTimelineReplace',
            value: new RealtimeProjectionRoomTimelineReplace({
              roomId,
              page: new RoomTimelinePage()
            })
          }
        })
      ]
    })
  });
}

function transientFrame(id = 'evt-1'): RealtimeServerFrame {
  return serverFrame({
    case: 'event',
    value: new RealtimeEventEnvelope({
      id,
      createdAt: Timestamp.now(),
      actorId: 'user-1',
      event: {
        case: 'userTyping',
        value: new RealtimeTypingEvent({ roomId: 'room-1' })
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

async function startAndSubscribeWithHeartbeatInterval(heartbeatIntervalSeconds: number): Promise<{
  fake: FakeServerConnection;
  socket: FakeRealtimeSocket;
}> {
  const fake = new FakeServerConnection();
  eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
  const socket = sockets.at(-1);
  if (!socket) throw new Error('expected realtime socket');
  socket.open();
  await socket.receive(helloFrame(heartbeatIntervalSeconds));
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
    eventBusManager.stopAll();
    setRealtimeSocketFactoryForTests(null);
    setRealtimePollRandomForTests(null);
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
    const hello = RealtimeClientFrame.fromBinary(sockets[0].sent[0]);
    expect(hello.frame.case).toBe('hello');
    if (hello.frame.case !== 'hello') throw new Error('expected hello frame');
    expect(hello.frame.value.protocolVersion).toBe(2);

    await sockets[0].receive(helloFrame());
    expect(sockets[0].sent).toHaveLength(2);
    await sockets[0].receive(subscribedFrame());
    expect(fake.status).toBe('connected');
  });

  it('requests a room timeline lazily and retains it across reconnect subscriptions', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-lazy');

    expect(socket.sent).toHaveLength(3);
    const hydration = RealtimeClientFrame.fromBinary(socket.sent[2]);
    expect(hydration.frame.case).toBe('hydrateRoom');
    if (hydration.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room frame');
    expect(hydration.frame.value.roomId).toBe('room-lazy');
    expect(sync.desiredRoomIds).toEqual(['room-lazy']);
    expect(sync.retainedRoomIds).toEqual([]);

    // Repeated selectors do not ask the server to rebuild the same timeline.
    eventBusManager.hydrateRoom(TEST_SERVER, 'room-lazy');
    expect(socket.sent).toHaveLength(3);

    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(roomTimelineFrame('room-lazy'));
    expect(sync.retainedRoomIds).toEqual(['room-lazy']);
    await socket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'cursor-lazy' }) })
    );
    socket.serverClose();
    await vi.advanceTimersByTimeAsync(0);

    const resumed = sockets.at(-1)!;
    resumed.open();
    await resumed.receive(helloFrame());
    const subscribe = RealtimeClientFrame.fromBinary(resumed.sent[1]);
    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.resumeCursor).toBe('cursor-lazy');
    expect(subscribe.frame.value.retainedRoomIds).toEqual(['room-lazy']);
  });

  it('includes materialized rooms in the first subscription', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.retainRoom('room-from-route');
    sync.confirmRoom('room-from-route');
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());

    const subscribe = RealtimeClientFrame.fromBinary(socket.sent[1]);
    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.retainedRoomIds).toEqual(['room-from-route']);
  });

  it('flushes a room retained between subscribe and subscribed', async () => {
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-during-handshake');
    expect(socket.sent).toHaveLength(2);

    await socket.receive(subscribedFrame());
    expect(socket.sent).toHaveLength(3);
    const hydration = RealtimeClientFrame.fromBinary(socket.sent[2]);
    expect(hydration.frame.case).toBe('hydrateRoom');
    if (hydration.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room frame');
    expect(hydration.frame.value.roomId).toBe('room-during-handshake');
  });

  it('retries a desired room after reconnect when its hydration response was lost', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-lost-response');
    expect(sync.desiredRoomIds).toEqual(['room-lost-response']);
    expect(sync.retainedRoomIds).toEqual([]);
    socket.serverClose();
    await vi.advanceTimersByTimeAsync(0);

    const resumed = sockets.at(-1)!;
    resumed.open();
    await resumed.receive(helloFrame());
    const subscribe = RealtimeClientFrame.fromBinary(resumed.sent[1]);
    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.retainedRoomIds).toEqual([]);

    await resumed.receive(subscribedFrame());
    const hydration = RealtimeClientFrame.fromBinary(resumed.sent[2]);
    expect(hydration.frame.case).toBe('hydrateRoom');
    if (hydration.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room frame');
    expect(hydration.frame.value.roomId).toBe('room-lost-response');
  });

  it('retries pending room hydration after a reconnectable server close frame', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-close-response');
    expect(socket.sent).toHaveLength(3);
    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: 'stream_closed',
          message: 'reconnect to continue',
          reconnect: true,
          retryAfterMs: 0
        })
      })
    );
    await vi.advanceTimersByTimeAsync(0);

    const resumed = sockets.at(-1)!;
    expect(resumed).not.toBe(socket);
    resumed.open();
    await resumed.receive(helloFrame());
    await resumed.receive(subscribedFrame());

    const hydration = RealtimeClientFrame.fromBinary(resumed.sent[2]);
    expect(hydration.frame.case).toBe('hydrateRoom');
    if (hydration.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room frame');
    expect(hydration.frame.value.roomId).toBe('room-close-response');
  });

  it('retries the rejected room after the server hydration backoff', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-rate-limited');
    expect(socket.sent).toHaveLength(3);
    await socket.receive(
      serverFrame({
        case: 'error',
        value: new RealtimeError({
          code: 'room_hydration_rate_limited',
          message: 'try again later',
          roomId: 'room-rate-limited',
          retryAfterMs: 250
        })
      })
    );

    eventBusManager.hydrateRoom(TEST_SERVER, 'room-rate-limited');
    await vi.advanceTimersByTimeAsync(249);
    expect(socket.sent).toHaveLength(3);
    await vi.advanceTimersByTimeAsync(1);

    expect(socket.sent).toHaveLength(4);
    const retry = RealtimeClientFrame.fromBinary(socket.sent[3]);
    expect(retry.frame.case).toBe('hydrateRoom');
    if (retry.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room retry');
    expect(retry.frame.value.roomId).toBe('room-rate-limited');

    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(roomTimelineFrame('room-rate-limited'));
    expect(sync.retainedRoomIds).toEqual(['room-rate-limited']);
  });

  it('serializes lazy room hydrations and advances after confirmation', async () => {
    const { socket } = await startAndSubscribe();
    eventBusManager.hydrateRoom(TEST_SERVER, 'room-first');
    eventBusManager.hydrateRoom(TEST_SERVER, 'room-second');

    expect(socket.sent).toHaveLength(3);
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(roomTimelineFrame('room-first'));

    expect(socket.sent).toHaveLength(4);
    const second = RealtimeClientFrame.fromBinary(socket.sent[3]);
    expect(second.frame.case).toBe('hydrateRoom');
    if (second.frame.case !== 'hydrateRoom') throw new Error('expected second hydrate room frame');
    expect(second.frame.value.roomId).toBe('room-second');
  });

  it('rolls the socket when LRU retention makes room for another timeline', async () => {
    const sync = new RealtimeProjectionSyncState();
    for (let index = 0; index < MAX_RETAINED_ROOM_TIMELINES; index++) {
      const roomId = `R${index}`;
      sync.retainRoom(roomId);
      sync.confirmRoom(roomId);
    }
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const original = sockets[0];
    original.open();
    await original.receive(helloFrame());
    await original.receive(subscribedFrame());

    eventBusManager.hydrateRoom(TEST_SERVER, 'R-overflow');

    expect(original.closeCalls.at(-1)?.reason).toBe('room retention rollover');
    expect(sockets).toHaveLength(2);
    const replacement = sockets[1];
    replacement.open();
    await replacement.receive(helloFrame());
    const subscribe = RealtimeClientFrame.fromBinary(replacement.sent[1]);
    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.retainedRoomIds).not.toContain('R0');
    expect(subscribe.frame.value.retainedRoomIds).not.toContain('R-overflow');
    expect(subscribe.frame.value.retainedRoomIds).toHaveLength(MAX_RETAINED_ROOM_TIMELINES - 1);

    await replacement.receive(subscribedFrame());
    const hydration = RealtimeClientFrame.fromBinary(replacement.sent[2]);
    expect(hydration.frame.case).toBe('hydrateRoom');
    if (hydration.frame.case !== 'hydrateRoom') throw new Error('expected hydrate room frame');
    expect(hydration.frame.value.roomId).toBe('R-overflow');
  });

  it('registers the bus but defers the socket until projection support is confirmed', () => {
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, false);

    expect(eventBusManager.getBus(TEST_SERVER)).toBeDefined();
    expect(sockets).toHaveLength(0);

    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true);

    expect(sockets).toHaveLength(1);
  });

  it('dispatches protobuf realtime events to existing event handlers', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    await socket.receive(transientFrame());

    expect(handler).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'evt-1',
        event: expect.objectContaining({
          kind: RoomEventKind.UserTyping,
          roomId: 'room-1'
        })
      })
    );
    expect(consoleDebug).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] event dispatched`,
      RoomEventKind.UserTyping,
      expect.objectContaining({ eventId: 'evt-1' })
    );
  });

  it('resumes socket reconnects only after the projection reducer applied the cursor', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe();
    const projectionHandler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(projectionHandler);

    await socket.receive(projectionFrame('cursor-applied'));
    expect(projectionHandler).toHaveBeenCalledTimes(1);
    await socket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'cursor-boundary' }) })
    );
    socket.serverClose();
    await vi.advanceTimersByTimeAsync(0);

    const resumed = sockets.at(-1)!;
    resumed.open();
    await resumed.receive(helloFrame());
    const subscribeFrame = RealtimeClientFrame.fromBinary(resumed.sent[1]);
    expect(subscribeFrame.frame.case).toBe('subscribeEvents');
    if (subscribeFrame.frame.case !== 'subscribeEvents')
      throw new Error('expected subscribe frame');
    expect(subscribeFrame.frame.value.resumeCursor).toBe('cursor-boundary');
  });

  it('accepts a compacted reset when a retained resume cursor is no longer usable', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.retainRoom('room-retained');
    sync.confirmRoom('room-retained');
    sync.markCaughtUp('cursor-expired');
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());

    await socket.receive(projectionFrame(undefined));

    expect(sync.phase).toBe('hydrating');
    // Snapshot frames are deliberately cursorless. Until the complete reset
    // reaches caught_up, reconnecting must retry the old cursor and receive a
    // fresh reset rather than resume from a partially rebuilt projection.
    expect(sync.resumeCursor).toBe('cursor-expired');
    expect(sync.desiredRoomIds).toEqual(['room-retained']);
    expect(sync.retainedRoomIds).toEqual([]);

    await socket.receive(roomTimelineFrame('room-retained'));
    expect(sync.resumeCursor).toBe('cursor-expired');
    await socket.receive(
      serverFrame({
        case: 'caughtUp',
        value: new RealtimeCaughtUp({ cursor: 'cursor-reset-caught-up' })
      })
    );

    expect(sync.phase).toBe('ready');
    expect(sync.resumeCursor).toBe('cursor-reset-caught-up');
    expect(sync.retainedRoomIds).toEqual(['room-retained']);
  });

  it('does not advance the cursor when no projection reducer is registered', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe();

    await socket.receive(projectionFrame('cursor-must-not-persist'));
    expect(socket.closeCalls.at(-1)?.reason).toBe('projection reducer failed');
    expect(consoleError).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] projection reducer failed`,
      expect.any(Error)
    );
  });

  it('closes and reconnects without advancing after an undecodable frame', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());
    await socket.receive(subscribedFrame());
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());

    await socket.receiveBytes(new Uint8Array([0xff, 0xff]));

    expect(socket.closeCalls.at(-1)?.reason).toBe('invalid realtime frame');
    expect(sync.resumeCursor).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('closes and reconnects without advancing after an unknown server frame', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(helloFrame());

    // Valid protobuf containing unknown length-delimited top-level field 99.
    await socket.receiveBytes(new Uint8Array([0x9a, 0x06, 0x00]));

    expect(socket.closeCalls.at(-1)?.reason).toBe('unsupported realtime frame');
    expect(sync.resumeCursor).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('preserves transient event display data in dispatched envelopes', async () => {
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
    expect(dispatched.event).toEqual(
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

    await socket.receive(transientFrame());

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

    await socket.receive(transientFrame('evt-1'));
    await socket.receive(transientFrame('evt-2'));

    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('marks the projection stale and reconnects when the socket closes', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();

    socket.serverClose();

    expect(fake.status).toBe('disconnected');
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('does not reconnect when the realtime stream reports authentication required', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();

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
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(1);
  });

  it('does not reconnect when the server rejects projection protocol v2', async () => {
    vi.useFakeTimers();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
    const socket = sockets[0];
    socket.open();

    await socket.receive(
      serverFrame({
        case: 'error',
        value: new RealtimeError({
          code: 'unsupported_protocol',
          message: 'unsupported realtime protocol version',
          fatal: true
        })
      })
    );

    expect(fake.status).toBe('disconnected');
    expect(socket.closeCalls.at(-1)?.reason).toBe('unsupported_protocol');
    await vi.advanceTimersByTimeAsync(60_000);
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

  it('reconnects when the ServerConnection retry bridge requests it', async () => {
    vi.useFakeTimers();
    const { fake } = await startAndSubscribe();

    fake.forceReconnect('user retry');

    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('reconnects when heartbeats stall', async () => {
    vi.useFakeTimers();
    await startAndSubscribeWithHeartbeatInterval(15);

    await vi.advanceTimersByTimeAsync(44_999);

    expect(sockets).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(sockets).toHaveLength(2);
  });

  it('falls back to the previous stall timeout when heartbeat interval is omitted', async () => {
    vi.useFakeTimers();
    await startAndSubscribeWithHeartbeatInterval(0);

    await vi.advanceTimersByTimeAsync(74_999);

    expect(sockets).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(sockets).toHaveLength(2);
  });

  it('does not dispatch heartbeat frames to handlers', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.handlers.add(handler);

    await socket.receive(heartbeatFrame());

    expect(handler).not.toHaveBeenCalled();
  });

  it('does NOT reconnect when stopBus is called', async () => {
    await startAndSubscribe();
    expect(sockets).toHaveLength(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].closeCalls).toHaveLength(1);
  });
  it('keeps only the active server live and closes an inactive catch-up at caught_up', async () => {
    const active = new FakeServerConnection();
    const inactive = new FakeServerConnection();
    inactive.realtimeUrl = 'ws://inactive.test/api/realtime';
    const activeSync = new RealtimeProjectionSyncState();
    const inactiveSync = new RealtimeProjectionSyncState();

    eventBusManager.synchronizeAuthenticatedServers(
      [
        {
          serverId: 'active-server',
          connection: active as unknown as ServerConnection,
          projectionSupported: true,
          sync: activeSync
        },
        {
          serverId: 'inactive-server',
          connection: inactive as unknown as ServerConnection,
          projectionSupported: true,
          sync: inactiveSync
        }
      ],
      'active-server'
    );
    eventBusManager.getBus('active-server')!.projectionHandlers.add(vi.fn());
    eventBusManager.getBus('inactive-server')!.projectionHandlers.add(vi.fn());

    expect(sockets.map((socket) => socket.url)).toEqual([active.realtimeUrl, inactive.realtimeUrl]);
    const inactiveSocket = sockets[1];
    inactiveSocket.open();
    await inactiveSocket.receive(helloFrame());
    await inactiveSocket.receive(projectionFrame('inactive-event'));
    await inactiveSocket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'inactive-ready' }) })
    );

    expect(inactiveSocket.closeCalls.at(-1)?.reason).toBe('caught_up');
    expect(inactiveSync.phase).toBe('stale');
    expect(inactiveSync.resumeCursor).toBe('inactive-ready');
    expect(active.status).toBe('connecting');
    expect(inactive.status).toBe('dormant');
  });

  it('reuses an inactive projection cursor when that server becomes active', async () => {
    const first = new FakeServerConnection();
    const second = new FakeServerConnection();
    second.realtimeUrl = 'ws://second.test/api/realtime';
    const firstSync = new RealtimeProjectionSyncState();
    const secondSync = new RealtimeProjectionSyncState();
    const registrations = [
      {
        serverId: 'first-server',
        connection: first as unknown as ServerConnection,
        projectionSupported: true,
        sync: firstSync
      },
      {
        serverId: 'second-server',
        connection: second as unknown as ServerConnection,
        projectionSupported: true,
        sync: secondSync
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'first-server');
    eventBusManager.getBus('first-server')!.projectionHandlers.add(vi.fn());
    eventBusManager.getBus('second-server')!.projectionHandlers.add(vi.fn());
    const firstLive = sockets[0];
    firstLive.open();
    await firstLive.receive(helloFrame());
    await firstLive.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'first-ready' }) })
    );
    const inactivePoll = sockets[1];
    inactivePoll.open();
    await inactivePoll.receive(helloFrame());
    await inactivePoll.receive(projectionFrame('second-event'));
    await inactivePoll.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'second-ready' }) })
    );

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'second-server');
    expect(firstLive.closeCalls.at(-1)?.reason).toBe('dormant');
    const promoted = sockets.at(-1)!;
    promoted.open();
    await promoted.receive(helloFrame());
    const subscribe = RealtimeClientFrame.fromBinary(promoted.sent[1]);

    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.resumeCursor).toBe('second-ready');
    expect(firstSync.phase).toBe('stale');
  });

  it('cancels a polling timeout when an in-flight poll is promoted to live', async () => {
    vi.useFakeTimers();
    const active = new FakeServerConnection();
    const promotedConnection = new FakeServerConnection();
    promotedConnection.realtimeUrl = 'ws://promoted.test/api/realtime';
    const registrations = [
      {
        serverId: 'active-before-promotion',
        connection: active as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState()
      },
      {
        serverId: 'promoted-server',
        connection: promotedConnection as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'active-before-promotion');
    for (const registration of registrations) {
      eventBusManager.getBus(registration.serverId)!.projectionHandlers.add(vi.fn());
    }
    const pollingSocket = sockets[1];
    pollingSocket.open();
    await pollingSocket.receive(helloFrame());
    await pollingSocket.receive(subscribedFrame());

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'promoted-server');
    expect(promotedConnection.status).toBe('connected');
    pollingSocket.serverClose();
    await vi.advanceTimersByTimeAsync(0);
    const replacement = sockets.at(-1)!;
    expect(replacement).not.toBe(pollingSocket);
    replacement.open();
    await replacement.receive(helloFrame(100));
    await replacement.receive(subscribedFrame());

    await vi.advanceTimersByTimeAsync(30_000);
    expect(replacement.closeCalls).toHaveLength(0);
    expect(promotedConnection.status).toBe('connected');
  });

  it('serializes initial catch-up connections for multiple inactive servers', async () => {
    const active = new FakeServerConnection();
    const inactiveA = new FakeServerConnection();
    const inactiveB = new FakeServerConnection();
    inactiveA.realtimeUrl = 'ws://inactive-a.test/api/realtime';
    inactiveB.realtimeUrl = 'ws://inactive-b.test/api/realtime';
    const registrations = [
      {
        serverId: 'active',
        connection: active as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState()
      },
      {
        serverId: 'inactive-a',
        connection: inactiveA as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState()
      },
      {
        serverId: 'inactive-b',
        connection: inactiveB as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'active');
    for (const registration of registrations) {
      eventBusManager.getBus(registration.serverId)!.projectionHandlers.add(vi.fn());
    }
    expect(sockets.map((socket) => socket.url)).toEqual([
      active.realtimeUrl,
      inactiveA.realtimeUrl
    ]);

    const pollA = sockets[1];
    pollA.open();
    await pollA.receive(helloFrame());
    await pollA.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'a-ready' }) })
    );
    await vi.waitFor(() => expect(sockets).toHaveLength(3));
    expect(sockets[2].url).toBe(inactiveB.realtimeUrl);
  });

  it('periodically resumes a ready inactive projection with jittered serialized polling', async () => {
    vi.useFakeTimers();
    setRealtimePollRandomForTests(() => 0.5);
    const active = new FakeServerConnection();
    const inactive = new FakeServerConnection();
    inactive.realtimeUrl = 'ws://periodic.test/api/realtime';
    const inactiveSync = new RealtimeProjectionSyncState();
    inactiveSync.markCaughtUp('periodic-cursor');

    eventBusManager.synchronizeAuthenticatedServers(
      [
        {
          serverId: 'periodic-active',
          connection: active as unknown as ServerConnection,
          projectionSupported: true,
          sync: new RealtimeProjectionSyncState()
        },
        {
          serverId: 'periodic-inactive',
          connection: inactive as unknown as ServerConnection,
          projectionSupported: true,
          sync: inactiveSync
        }
      ],
      'periodic-active'
    );
    eventBusManager.getBus('periodic-active')!.projectionHandlers.add(vi.fn());
    eventBusManager.getBus('periodic-inactive')!.projectionHandlers.add(vi.fn());

    expect(sockets).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(59_999);
    expect(sockets).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(sockets).toHaveLength(2);

    const poll = sockets[1];
    poll.open();
    await poll.receive(helloFrame());
    const subscribe = RealtimeClientFrame.fromBinary(poll.sent[1]);
    expect(subscribe.frame.case).toBe('subscribeEvents');
    if (subscribe.frame.case !== 'subscribeEvents') throw new Error('expected subscribe frame');
    expect(subscribe.frame.value.resumeCursor).toBe('periodic-cursor');
  });
});
