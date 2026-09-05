import { Timestamp } from '@bufbuild/protobuf';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  RealtimeEvent,
  RealtimeClose,
  RealtimeCaughtUp,
  RealtimeHeartbeat,
  RealtimeServerFrame,
  RealtimeSnapshot,
  RealtimeSubscribe,
  RealtimeCloseCode
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { UserTypingEvent } from '@chatto/api-types/realtime/v1/events_pb';
import {
  eventBusManager,
  setRealtimePollRandomForTests,
  setRealtimeSocketFactoryForTests
} from './eventBus.svelte';
import type { ConnectionStatus, ServerConnection } from './serverConnection.svelte';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';

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
    for (let index = 0; index < 8; index++) await Promise.resolve();
  }

  async receiveBytes(data: Uint8Array): Promise<void> {
    this.onmessage?.({ data });
    for (let index = 0; index < 8; index++) await Promise.resolve();
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
  browserRenewalCalls = 0;
  authRenewed = false;
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

  async handleAuthenticationRequired(): Promise<boolean> {
    this.authRequiredCalls++;
    if (this.authRenewed) this.bearerToken = 'token-2';
    return this.authRenewed;
  }

  async renewBrowserSession(): Promise<boolean> {
    this.browserRenewalCalls++;
    return true;
  }
}

const TEST_SERVER = 'test-server-bus';
let sockets: FakeRealtimeSocket[];

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

async function flushPromises(): Promise<void> {
  for (let index = 0; index < 8; index++) await Promise.resolve();
}

function serverFrame(frame: RealtimeServerFrame['frame']): RealtimeServerFrame {
  return new RealtimeServerFrame({ frame });
}

function snapshotFrame(): RealtimeServerFrame {
  return serverFrame({
    case: 'snapshot',
    value: new RealtimeSnapshot({
      server: new ServerPublicProfile({ name: 'Snapshot Server' })
    })
  });
}

function projectionFrame(cursor: string | undefined): RealtimeServerFrame {
  return serverFrame({
    case: 'event',
    value: new RealtimeEvent({
      cursor
    })
  });
}

function cursorlessFrame(id = 'evt-1'): RealtimeServerFrame {
  return serverFrame({
    case: 'event',
    value: new RealtimeEvent({
      id,
      createdAt: Timestamp.now(),
      actorId: 'user-1',
      event: {
        case: 'userTyping',
        value: new UserTypingEvent({ roomId: 'room-1' })
      }
    })
  });
}

function heartbeatFrame(resumeCursor?: string): RealtimeServerFrame {
  return serverFrame({
    case: 'heartbeat',
    value: new RealtimeHeartbeat({
      cursor: resumeCursor
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

  it('opens /api/realtime and sends one complete subscription', async () => {
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].url).toBe(fake.realtimeUrl);
    sockets[0].open();
    expect(sockets[0].sent).toHaveLength(1);
    const subscribe = RealtimeSubscribe.fromBinary(sockets[0].sent[0]);
    expect(subscribe.protocolVersion).toBe(4);
    expect(subscribe.bearerToken).toBe('token-1');
    expect(subscribe.initialState).toBe(2);
    expect(fake.status).toBe('connecting');
    await sockets[0].receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'ready' }) })
    );
    expect(fake.status).toBe('connected');
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

    await socket.receive(cursorlessFrame());

    expect(handler).toHaveBeenCalledWith(
      expect.objectContaining({
        id: 'evt-1',
        event: expect.objectContaining({ case: 'userTyping' })
      })
    );
    expect(consoleDebug).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] event dispatched`,
      'userTyping',
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
    const subscribe = RealtimeSubscribe.fromBinary(resumed.sent[0]);
    expect(subscribe.resumeCursor).toBe('cursor-boundary');
  });

  it('replaces retained state when a resume cursor falls back to a snapshot', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('cursor-expired');
    const fake = new FakeServerConnection();
    const completeProjectionCatchUp = vi.fn().mockResolvedValue(undefined);
    eventBusManager.startBus(
      TEST_SERVER,
      fake as unknown as ServerConnection,
      true,
      sync,
      completeProjectionCatchUp
    );
    const socket = sockets[0];
    socket.open();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(snapshotFrame());

    expect(sync.phase).toBe('hydrating');
    expect(sync.resumeCursor).toBeNull();

    await socket.receive(
      serverFrame({
        case: 'caughtUp',
        value: new RealtimeCaughtUp({ cursor: 'cursor-reset-caught-up' })
      })
    );

    expect(sync.phase).toBe('ready');
    expect(sync.resumeCursor).toBe('cursor-reset-caught-up');
    expect(completeProjectionCatchUp).toHaveBeenCalledWith('cursor-reset-caught-up');
    expect(fake.status).toBe('connected');
  });

  it('rejects a second snapshot on the same subscription', async () => {
    const { socket } = await startAndSubscribe();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());

    await socket.receive(snapshotFrame());
    await socket.receive(snapshotFrame());

    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('invalid snapshot frame');
  });

  it('rejects an atomic snapshot without its server profile', async () => {
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(
      serverFrame({
        case: 'snapshot',
        value: new RealtimeSnapshot()
      })
    );

    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('invalid snapshot frame');
    expect(sync.resumeCursor).toBeNull();
  });

  it('rejects snapshot recovery before a projection reducer is registered', async () => {
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
    await socket.receive(snapshotFrame());

    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('snapshot reducer failed');
    expect(sync.resumeCursor).toBeNull();
  });

  it('does not advance the cursor when no projection reducer is registered', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe();

    await socket.receive(projectionFrame('cursor-must-not-persist'));
    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('projection reducer failed');
    expect(consoleError).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] projection reducer failed`,
      expect.any(Error)
    );
  });

  it('retains the last complete cursor when event resource reconciliation fails', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('cursor-before-failure');
    const fake = new FakeServerConnection();
    const completeProjectionCatchUp = vi.fn((cursor: string) =>
      cursor === 'cursor-failed'
        ? Promise.reject(new Error('resource read failed'))
        : Promise.resolve()
    );
    eventBusManager.startBus(
      TEST_SERVER,
      fake as unknown as ServerConnection,
      true,
      sync,
      completeProjectionCatchUp
    );
    const socket = sockets[0];
    socket.open();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());

    await socket.receive(projectionFrame('cursor-failed'));

    expect(socket.closeCalls.at(-1)?.reason).toBe('resource reconciliation failed');
    expect(sync.resumeCursor).toBe('cursor-before-failure');
  });

  it('closes and reconnects without advancing after an undecodable frame', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection, true, sync);
    const socket = sockets[0];
    socket.open();
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

    // Valid protobuf containing unknown length-delimited top-level field 99.
    await socket.receiveBytes(new Uint8Array([0x9a, 0x06, 0x00]));

    expect(socket.closeCalls.at(-1)?.reason).toBe('unsupported realtime frame');
    expect(sync.resumeCursor).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
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

    await socket.receive(cursorlessFrame());

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
  });

  it('continues delivering events after a handler error on a previous event', async () => {
    const { socket } = await startAndSubscribe();
    const handler = vi.fn();
    let throwOnce = true;
    const bus = eventBusManager.getBus(TEST_SERVER)!;
    bus.projectionHandlers.add(vi.fn());
    bus.handlers.add(() => {
      if (throwOnce) {
        throwOnce = false;
        throw new Error('handler boom');
      }
    });
    bus.handlers.add(handler);

    await socket.receive(cursorlessFrame('evt-1'));
    await socket.receive(cursorlessFrame('evt-2'));

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

  it('reconnects when the server reports temporary unavailability', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe();

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.TEMPORARILY_UNAVAILABLE,
          message: 'realtime replay is temporarily unavailable',
          reconnect: true
        })
      })
    );

    expect(socket.closeCalls.at(-1)).toEqual({
      code: 1000,
      reason: 'realtime replay is temporarily unavailable'
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('does not reconnect when the server rejects the older protocol version', async () => {
    vi.useFakeTimers();
    const fake = new FakeServerConnection();
    eventBusManager.startBus(TEST_SERVER, fake as unknown as ServerConnection);
    const socket = sockets[0];
    socket.open();

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.UNSUPPORTED_PROTOCOL,
          message: 'unsupported realtime protocol version',
          reconnect: false
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
          code: RealtimeCloseCode.AUTHENTICATION_REQUIRED,
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

  it('dispatches session termination as control state and does not reconnect', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();
    const handler = vi.fn();
    eventBusManager.getBus(TEST_SERVER)!.sessionTerminatedHandlers.add(handler);

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.SESSION_TERMINATED,
          message: 'session terminated: admin_boot',
          reconnect: false
        })
      })
    );

    expect(handler).toHaveBeenCalledWith('session terminated: admin_boot');
    expect(fake.status).toBe('disconnected');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(sockets).toHaveLength(1);
  });

  it('renews and reconnects in place when the access token expires', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();
    fake.authRenewed = true;

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.AUTHENTICATION_REQUIRED,
          message: 'access token expired',
          reconnect: true
        })
      })
    );
    await vi.advanceTimersByTimeAsync(0);

    expect(fake.authRequiredCalls).toBe(1);
    expect(sockets).toHaveLength(2);
    sockets[1].open();
    const subscribe = RealtimeSubscribe.fromBinary(sockets[1].sent[0]);
    expect(subscribe.bearerToken).toBe('token-2');
  });

  it('reconnects cookie sessions when the server requests automatic renewal', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();

    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.SESSION_RENEWAL_REQUIRED,
          message: 'browser session ready for renewal',
          reconnect: true
        })
      })
    );
    await vi.advanceTimersByTimeAsync(0);

    expect(fake.authRequiredCalls).toBe(0);
    expect(fake.browserRenewalCalls).toBe(1);
    expect(sockets).toHaveLength(2);
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
    await startAndSubscribe();

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

  it('retains a heartbeat cursor only after earlier reconciliation completes', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('before-heartbeat');
    const reconciliation = deferred<void>();
    const completeProjectionCatchUp = vi.fn((cursor: string) =>
      cursor === 'heartbeat-cursor' ? reconciliation.promise : Promise.resolve()
    );
    const fake = new FakeServerConnection();
    eventBusManager.startBus(
      TEST_SERVER,
      fake as unknown as ServerConnection,
      true,
      sync,
      completeProjectionCatchUp
    );
    const socket = sockets[0];
    socket.open();
    eventBusManager.getBus(TEST_SERVER)!.projectionHandlers.add(vi.fn());
    await socket.receive(heartbeatFrame('heartbeat-cursor'));

    expect(sync.resumeCursor).toBe('before-heartbeat');
    reconciliation.resolve();
    await flushPromises();
    expect(sync.resumeCursor).toBe('heartbeat-cursor');
  });

  it('does NOT reconnect when stopBus is called', async () => {
    await startAndSubscribe();
    expect(sockets).toHaveLength(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].closeCalls).toHaveLength(1);
  });

  it('installs a registered projection reducer before opening its transport', async () => {
    const connection = new FakeServerConnection();
    const sync = new RealtimeProjectionSyncState();
    const projectionHandler = vi.fn();

    eventBusManager.synchronizeAuthenticatedServers(
      [
        {
          serverId: TEST_SERVER,
          connection: connection as unknown as ServerConnection,
          projectionSupported: true,
          sync,
          projectionHandler
        }
      ],
      TEST_SERVER
    );

    const socket = sockets[0];
    socket.open();
    await socket.receive(projectionFrame('initial-projection'));

    expect(projectionHandler).toHaveBeenCalledOnce();
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
    await firstLive.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'first-ready' }) })
    );
    const inactivePoll = sockets[1];
    inactivePoll.open();
    await inactivePoll.receive(projectionFrame('second-event'));
    await inactivePoll.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'second-ready' }) })
    );

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'second-server');
    expect(firstLive.closeCalls.at(-1)?.reason).toBe('dormant');
    const promoted = sockets.at(-1)!;
    promoted.open();
    const subscribe = RealtimeSubscribe.fromBinary(promoted.sent[0]);

    expect(subscribe.resumeCursor).toBe('second-ready');
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
    await pollingSocket.receive(heartbeatFrame());

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'promoted-server');
    expect(promotedConnection.status).toBe('connecting');
    pollingSocket.serverClose();
    await vi.advanceTimersByTimeAsync(0);
    const replacement = sockets.at(-1)!;
    expect(replacement).not.toBe(pollingSocket);
    replacement.open();
    await replacement.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'promoted-ready' }) })
    );

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
    const subscribe = RealtimeSubscribe.fromBinary(poll.sent[0]);
    expect(subscribe.resumeCursor).toBe('periodic-cursor');
  });
});
