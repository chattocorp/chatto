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
import type { EventBus, ProjectionHandler } from '$lib/eventBus.svelte';

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
  #connectionFailed = false;

  setRealtimeConnectionStatus(status: ConnectionStatus): void {
    if (status === 'disconnected') this.#connectionFailed = true;
    if (status === 'connected' || status === 'dormant') this.#connectionFailed = false;
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

  get showConnectionLostIcon(): boolean {
    return this.#connectionFailed;
  }

  forceReconnect(reason: string): void {
    this.#connectionFailed = false;
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

/** Options for {@link startLiveBus}; each maps to one `ensureBus` parameter. */
type LiveBusOptions = {
  projectionSupported?: boolean;
  sync?: RealtimeProjectionSyncState;
  reducer?: ProjectionHandler;
  completeProjectionCatchUp?: (cursor: string) => Promise<void>;
  waitForProjectionReconciliation?: () => Promise<void>;
};

/**
 * Register the test server's bus and, when projection is supported, make its
 * transport live. This is what the app coordinator does for the active server.
 */
function startLiveBus(fake: FakeServerConnection, options: LiveBusOptions = {}): EventBus {
  const projectionSupported = options.projectionSupported ?? true;
  const controller = eventBusManager.ensureBus({
    serverId: TEST_SERVER,
    connection: fake as unknown as ServerConnection,
    projectionSupported,
    sync: options.sync ?? new RealtimeProjectionSyncState(),
    projectionHandler: options.reducer ?? (() => {}),
    completeProjectionCatchUp: options.completeProjectionCatchUp,
    waitForProjectionReconciliation: options.waitForProjectionReconciliation
  });
  if (projectionSupported) controller.setMode('live');
  return eventBusManager.getBus(TEST_SERVER)!;
}

async function startAndSubscribe(
  fake = new FakeServerConnection(),
  reducer?: ProjectionHandler
): Promise<{
  fake: FakeServerConnection;
  socket: FakeRealtimeSocket;
  bus: EventBus;
}> {
  const bus = startLiveBus(fake, { reducer });
  const socket = sockets.at(-1);
  if (!socket) throw new Error('expected realtime socket');
  socket.open();
  return { fake, socket, bus };
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
    startLiveBus(fake);

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
    startLiveBus(fake, { projectionSupported: false });

    expect(eventBusManager.getBus(TEST_SERVER)).toBeDefined();
    expect(sockets).toHaveLength(0);

    startLiveBus(fake, { projectionSupported: true });

    expect(sockets).toHaveLength(1);
  });

  it('dispatches protobuf realtime events to the reducer and then to listeners', async () => {
    const calls: string[] = [];
    const reducer = vi.fn(() => calls.push('reducer'));
    const { socket, bus } = await startAndSubscribe(undefined, reducer);
    const listener = vi.fn(() => calls.push('listener'));
    bus.subscribe(listener);

    await socket.receive(cursorlessFrame());

    const expectedUpdate = expect.objectContaining({
      event: expect.objectContaining({
        id: 'evt-1',
        event: expect.objectContaining({ case: 'userTyping' })
      })
    });
    expect(reducer).toHaveBeenCalledWith(expectedUpdate);
    expect(listener).toHaveBeenCalledWith(expectedUpdate);
    expect(calls).toEqual(['reducer', 'listener']);
    expect(consoleDebug).toHaveBeenCalledWith(
      `[eventBus:${TEST_SERVER}] event dispatched`,
      'userTyping',
      expect.objectContaining({ eventId: 'evt-1' })
    );
  });

  it('resumes socket reconnects only after the projection reducer applied the cursor', async () => {
    vi.useFakeTimers();
    const projectionHandler = vi.fn();
    const { socket } = await startAndSubscribe(undefined, projectionHandler);

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

  it('completes privileged-mode refresh only after resource reconciliation without resetting state', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('retained-cursor');
    const requested = sync.invalidateAuthorization();
    const refreshed = sync.waitForAuthorizationRefresh(requested);
    let finish!: () => void;
    const reconcile = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        })
    );
    const fake = new FakeServerConnection();
    const updates = vi.fn();
    startLiveBus(fake, { sync, reducer: updates, completeProjectionCatchUp: reconcile });
    const socket = sockets[0];
    socket.open();
    expect(RealtimeSubscribe.fromBinary(socket.sent[0]).resumeCursor).toBe('retained-cursor');
    await socket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'current-cursor' }) })
    );
    expect(sync.phase).toBe('stale');
    expect(sync.authorizationRefreshRequired).toBe(true);
    finish();
    await expect(refreshed).resolves.toBe(true);
    expect(reconcile).toHaveBeenCalledWith('current-cursor');
    expect(sync.authorizationRefreshRequired).toBe(false);
    expect(updates).not.toHaveBeenCalled();
  });

  it('invalidates privileged authority when the server deadline closes the socket', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('retained-cursor');
    const fake = new FakeServerConnection();
    startLiveBus(fake, { sync });
    const socket = sockets[0];
    socket.open();
    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({
          code: RealtimeCloseCode.PRIVILEGED_MODE_EXPIRED,
          reconnect: true
        })
      })
    );
    expect(sync.authorizationRefreshRequired).toBe(true);
    expect(sync.hasUsableProjection).toBe(true);
    expect(sync.resumeCursor).toBe('retained-cursor');
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('purges on resync before reconnect even when an optional reset listener fails', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('rejected');
    const fake = new FakeServerConnection();
    const cleared = vi.fn();
    const bus = startLiveBus(fake, {
      sync,
      reducer: (update) => {
        if (update.privacyReset) cleared();
      }
    });
    const socket = sockets[0];
    socket.open();
    const listenerCleared = vi.fn();
    bus.subscribe(() => {
      throw new Error('optional mirror');
    });
    bus.subscribe((update) => {
      if (update.privacyReset) listenerCleared();
    });
    await socket.receive(
      serverFrame({
        case: 'close',
        value: new RealtimeClose({ code: RealtimeCloseCode.RESYNC_REQUIRED, reconnect: true })
      })
    );
    expect(cleared).toHaveBeenCalledOnce();
    expect(listenerCleared).toHaveBeenCalledOnce();
    expect(sync.resumeCursor).toBeNull();
    expect(sync.hasUsableProjection).toBe(false);
    expect(sockets).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(0);
    sockets[1].open();
    expect(RealtimeSubscribe.fromBinary(sockets[1].sent[0]).resumeCursor).toBeUndefined();
  });

  it('does not commit an event cursor after the client chooses a permission reload', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('old');
    const fake = new FakeServerConnection();
    startLiveBus(fake, {
      sync,
      reducer: (update) => {
        if (update.event?.event.case === 'viewerPermissionsChanged') sync.reset();
      }
    });
    sockets[0].open();
    await sockets[0].receive(
      serverFrame({
        case: 'event',
        value: new RealtimeEvent({
          cursor: 'must-not-resume',
          event: { case: 'viewerPermissionsChanged', value: {} }
        })
      })
    );
    expect(sync.resumeCursor).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    sockets[1].open();
    expect(RealtimeSubscribe.fromBinary(sockets[1].sent[0]).resumeCursor).toBeUndefined();
  });

  it('replaces retained state when a resume cursor falls back to a snapshot', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('cursor-expired');
    const fake = new FakeServerConnection();
    const completeProjectionCatchUp = vi.fn().mockResolvedValue(undefined);
    const projectionHandler = vi.fn();
    startLiveBus(fake, { sync, reducer: projectionHandler, completeProjectionCatchUp });
    const socket = sockets[0];
    socket.open();
    await socket.receive(snapshotFrame());

    expect(sync.phase).toBe('hydrating');
    expect(sync.resumeCursor).toBeNull();
    expect(projectionHandler).toHaveBeenCalledWith(
      expect.objectContaining({
        reset: true,
        retainView: true,
        privacyReset: false
      })
    );

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

  it('keeps the retained view when an interrupted warm snapshot retries', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('old-cursor');
    const fake = new FakeServerConnection();
    const updates = vi.fn();
    startLiveBus(fake, { sync, reducer: updates });
    const first = sockets[0];
    first.open();
    await first.receive(snapshotFrame());
    expect(sync.hasDisplayableView).toBe(true);
    first.serverClose();

    await vi.advanceTimersByTimeAsync(5_000);
    expect(sockets.length).toBeGreaterThan(1);
    const retry = sockets.at(-1)!;
    retry.open();
    await retry.receive(snapshotFrame());

    expect(updates).toHaveBeenCalledWith(
      expect.objectContaining({
        reset: true,
        retainView: true,
        privacyReset: false
      })
    );
    expect(sync.hasDisplayableView).toBe(true);
  });

  it('rejects a second snapshot on the same subscription', async () => {
    const { socket } = await startAndSubscribe(undefined, vi.fn());

    await socket.receive(snapshotFrame());
    await socket.receive(snapshotFrame());

    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('invalid snapshot frame');
  });

  it('rejects an atomic snapshot without its server profile', async () => {
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    startLiveBus(fake, { sync, reducer: vi.fn() });
    const socket = sockets[0];
    socket.open();
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

  it('rejects snapshot recovery when the projection reducer fails', async () => {
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    startLiveBus(fake, {
      sync,
      reducer: () => {
        throw new Error('reducer failed');
      }
    });
    const socket = sockets[0];
    socket.open();
    await socket.receive(snapshotFrame());

    expect(socket.closeCalls.at(-1)?.code).toBe(4000);
    expect(socket.closeCalls.at(-1)?.reason).toBe('snapshot reducer failed');
    expect(sync.resumeCursor).toBeNull();
  });

  it('does not advance the cursor when the projection reducer fails', async () => {
    vi.useFakeTimers();
    const { socket } = await startAndSubscribe(new FakeServerConnection(), () => {
      throw new Error('reducer failed');
    });

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
    const waitForReconciliation = vi.fn().mockRejectedValue(new Error('resource read failed'));
    startLiveBus(fake, {
      sync,
      reducer: vi.fn(),
      waitForProjectionReconciliation: waitForReconciliation
    });
    const socket = sockets[0];
    socket.open();

    await socket.receive(projectionFrame('cursor-failed'));

    expect(socket.closeCalls.at(-1)?.reason).toBe('resource reconciliation failed');
    expect(sync.resumeCursor).toBe('cursor-before-failure');
    expect(sync.phase).toBe('stale');
  });

  it('closes and reconnects without advancing after an undecodable frame', async () => {
    vi.useFakeTimers();
    const sync = new RealtimeProjectionSyncState();
    const fake = new FakeServerConnection();
    startLiveBus(fake, { sync, reducer: vi.fn() });
    const socket = sockets[0];
    socket.open();

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
    startLiveBus(fake, { sync });
    const socket = sockets[0];
    socket.open();

    // Valid protobuf containing unknown length-delimited top-level field 99.
    await socket.receiveBytes(new Uint8Array([0x9a, 0x06, 0x00]));

    expect(socket.closeCalls.at(-1)?.reason).toBe('unsupported realtime frame');
    expect(sync.resumeCursor).toBeNull();
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);
  });

  it('isolates listener errors so one throwing listener does not stop the others', async () => {
    const { socket, bus } = await startAndSubscribe(undefined, vi.fn());
    const ranBefore = vi.fn();
    const ranAfter = vi.fn();
    bus.subscribe(ranBefore);
    bus.subscribe(() => {
      throw new Error('handler boom');
    });
    bus.subscribe(ranAfter);

    await socket.receive(cursorlessFrame());

    expect(ranBefore).toHaveBeenCalledTimes(1);
    expect(ranAfter).toHaveBeenCalledTimes(1);
    expect(consoleError.mock.calls[0][0]).toContain('handler threw');
    expect(socket.closeCalls).toHaveLength(0);
  });

  it('continues delivering events after a listener error on a previous event', async () => {
    const { socket, bus } = await startAndSubscribe(undefined, vi.fn());
    const handler = vi.fn();
    let throwOnce = true;
    bus.subscribe(() => {
      if (throwOnce) {
        throwOnce = false;
        throw new Error('handler boom');
      }
    });
    bus.subscribe(handler);

    await socket.receive(cursorlessFrame('evt-1'));
    await socket.receive(cursorlessFrame('evt-2'));

    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('retries at once before it reports a lost connection', async () => {
    vi.useFakeTimers();
    const { fake, socket } = await startAndSubscribe();

    socket.serverClose();

    expect(fake.status).toBe('connecting');
    await vi.advanceTimersByTimeAsync(0);
    expect(sockets).toHaveLength(2);

    sockets[1].serverClose();

    expect(fake.status).toBe('disconnected');
    expect(fake.statusUpdates.filter((status) => status === 'disconnected')).toHaveLength(1);
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
    startLiveBus(fake);
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
    const { fake, socket, bus } = await startAndSubscribe();
    const handler = vi.fn();
    bus.onSessionTerminated(handler);

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
    expect(fake.statusUpdates).not.toContain('disconnected');
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
    expect(fake.statusUpdates).not.toContain('disconnected');
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

  it('does not dispatch heartbeat frames to the reducer or listeners', async () => {
    const reducer = vi.fn();
    const { socket, bus } = await startAndSubscribe(undefined, reducer);
    const handler = vi.fn();
    bus.subscribe(handler);

    await socket.receive(heartbeatFrame());

    expect(reducer).not.toHaveBeenCalled();
    expect(handler).not.toHaveBeenCalled();
  });

  it('retains a heartbeat cursor only after earlier reconciliation completes', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('before-heartbeat');
    const reconciliation = deferred<void>();
    const waitForReconciliation = vi.fn(() => reconciliation.promise);
    const fake = new FakeServerConnection();
    startLiveBus(fake, {
      sync,
      reducer: vi.fn(),
      waitForProjectionReconciliation: waitForReconciliation
    });
    const socket = sockets[0];
    socket.open();
    await socket.receive(heartbeatFrame('heartbeat-cursor'));

    expect(sync.resumeCursor).toBe('before-heartbeat');
    reconciliation.resolve();
    await flushPromises();
    expect(sync.resumeCursor).toBe('heartbeat-cursor');
  });

  it('does NOT reconnect when stopBus is called', async () => {
    const { fake } = await startAndSubscribe();
    expect(sockets).toHaveLength(1);

    eventBusManager.stopBus(TEST_SERVER);

    expect(sockets).toHaveLength(1);
    expect(sockets[0].closeCalls).toHaveLength(1);
    // A stopped transport is not a failed attempt.
    expect(fake.showConnectionLostIcon).toBe(false);
  });

  it('refreshes auxiliary state once per catch-up, not per replay event or heartbeat', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('before-replay');
    const completeCatchUp = vi.fn().mockResolvedValue(undefined);
    const pendingRead = deferred<void>();
    const waitForReconciliation = vi.fn(() => pendingRead.promise);
    startLiveBus(new FakeServerConnection(), {
      sync,
      reducer: vi.fn(),
      completeProjectionCatchUp: completeCatchUp,
      waitForProjectionReconciliation: waitForReconciliation
    });
    const socket = sockets[0];
    socket.open();
    for (let index = 0; index < 100; index++)
      await socket.receive(projectionFrame(`event-${index}`));
    await socket.receive(heartbeatFrame('heartbeat-before-caught-up'));
    await socket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'caught-up' }) })
    );
    expect(completeCatchUp).not.toHaveBeenCalled();
    expect(sync.resumeCursor).toBe('before-replay');
    pendingRead.resolve();
    await vi.waitFor(() => expect(sync.resumeCursor).toBe('caught-up'));
    expect(completeCatchUp).toHaveBeenCalledExactlyOnceWith('caught-up');
    await socket.receive(projectionFrame('live-event'));
    await socket.receive(heartbeatFrame('idle-heartbeat'));
    await vi.waitFor(() => expect(sync.resumeCursor).toBe('idle-heartbeat'));
    expect(completeCatchUp).toHaveBeenCalledTimes(1);
  });

  it('does not retain event or heartbeat cursors before snapshot hydration completes', async () => {
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('expired');
    const hydration = deferred<void>();
    const completeCatchUp = vi.fn(() => hydration.promise);
    const fake = new FakeServerConnection();
    startLiveBus(fake, {
      sync,
      reducer: vi.fn(),
      completeProjectionCatchUp: completeCatchUp,
      waitForProjectionReconciliation: async () => {}
    });
    const socket = sockets[0];
    socket.open();
    await socket.receive(snapshotFrame());
    await socket.receive(projectionFrame('snapshot-event'));
    await socket.receive(heartbeatFrame('snapshot-heartbeat'));
    expect(sync.resumeCursor).toBeNull();
    await socket.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'hydrated' }) })
    );
    expect(completeCatchUp).toHaveBeenCalledExactlyOnceWith('hydrated');
    expect(sync.resumeCursor).toBeNull();
    hydration.resolve();
    await vi.waitFor(() => expect(sync.resumeCursor).toBe('hydrated'));
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
          sync: activeSync,
          projectionHandler: vi.fn()
        },
        {
          serverId: 'inactive-server',
          connection: inactive as unknown as ServerConnection,
          projectionSupported: true,
          sync: inactiveSync,
          projectionHandler: vi.fn()
        }
      ],
      'active-server'
    );

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
        sync: firstSync,
        projectionHandler: vi.fn()
      },
      {
        serverId: 'second-server',
        connection: second as unknown as ServerConnection,
        projectionSupported: true,
        sync: secondSync,
        projectionHandler: vi.fn()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'first-server');
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
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      },
      {
        serverId: 'promoted-server',
        connection: promotedConnection as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'active-before-promotion');
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
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      },
      {
        serverId: 'inactive-a',
        connection: inactiveA as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      },
      {
        serverId: 'inactive-b',
        connection: inactiveB as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'active');
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

  it('immediately catches up a projection that became inactive before its first catch-up', async () => {
    const home = new FakeServerConnection();
    const remote = new FakeServerConnection();
    home.realtimeUrl = 'ws://home.test/api/realtime';
    remote.realtimeUrl = 'ws://remote.test/api/realtime';
    const homeSync = new RealtimeProjectionSyncState();
    homeSync.markCaughtUp('home-ready');
    const remoteSync = new RealtimeProjectionSyncState();
    const registrations = [
      {
        serverId: 'interrupted-home',
        connection: home as unknown as ServerConnection,
        projectionSupported: true,
        sync: homeSync,
        projectionHandler: vi.fn()
      },
      {
        serverId: 'interrupted-remote',
        connection: remote as unknown as ServerConnection,
        projectionSupported: true,
        sync: remoteSync,
        projectionHandler: vi.fn()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'interrupted-remote');
    sockets[0].open();
    await sockets[0].receive(snapshotFrame());
    expect(remoteSync.phase).toBe('hydrating');

    // Leave the remote server before its live socket reaches caught_up.
    eventBusManager.synchronizeAuthenticatedServers(registrations, 'interrupted-home');

    expect(sockets.map((socket) => socket.url)).toEqual([
      remote.realtimeUrl,
      home.realtimeUrl,
      remote.realtimeUrl
    ]);
    const poll = sockets[2];
    poll.open();
    await poll.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'remote-ready' }) })
    );
    expect(remoteSync.hasUsableProjection).toBe(true);
  });

  it('reruns an unready catch-up that arrives while another poll is in flight', async () => {
    const home = new FakeServerConnection();
    const remote = new FakeServerConnection();
    const other = new FakeServerConnection();
    home.realtimeUrl = 'ws://home.test/api/realtime';
    remote.realtimeUrl = 'ws://remote.test/api/realtime';
    other.realtimeUrl = 'ws://other.test/api/realtime';
    const homeSync = new RealtimeProjectionSyncState();
    homeSync.markCaughtUp('home-ready');
    const registrations = [
      {
        serverId: 'overlap-home',
        connection: home as unknown as ServerConnection,
        projectionSupported: true,
        sync: homeSync,
        projectionHandler: vi.fn()
      },
      {
        serverId: 'overlap-remote',
        connection: remote as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      },
      {
        serverId: 'overlap-other',
        connection: other as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      }
    ];

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'overlap-remote');
    expect(sockets.map((socket) => socket.url)).toEqual([remote.realtimeUrl, other.realtimeUrl]);

    // The remote server goes dormant mid-hydration while the other server's poll runs.
    eventBusManager.synchronizeAuthenticatedServers(registrations, 'overlap-home');
    expect(sockets).toHaveLength(3);
    expect(sockets[2].url).toBe(home.realtimeUrl);

    const otherPoll = sockets[1];
    otherPoll.open();
    await otherPoll.receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'other-ready' }) })
    );
    await vi.waitFor(() => expect(sockets).toHaveLength(4));
    expect(sockets[3].url).toBe(remote.realtimeUrl);
  });

  it('keeps the warning of a failed server when it becomes inactive', async () => {
    vi.useFakeTimers();
    const failing = new FakeServerConnection();
    failing.realtimeUrl = 'ws://failing.test/api/realtime';
    const other = new FakeServerConnection();
    other.realtimeUrl = 'ws://other.test/api/realtime';
    const registrations = [
      {
        serverId: 'failing-server',
        connection: failing as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      },
      {
        serverId: 'other-server',
        connection: other as unknown as ServerConnection,
        projectionSupported: true,
        sync: new RealtimeProjectionSyncState(),
        projectionHandler: vi.fn()
      }
    ];
    const failingSocket = () =>
      sockets.filter((socket) => socket.url === failing.realtimeUrl).at(-1)!;

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'failing-server');
    failingSocket().serverClose();
    await vi.advanceTimersByTimeAsync(0);
    failingSocket().serverClose();
    expect(failing.status).toBe('disconnected');

    eventBusManager.synchronizeAuthenticatedServers(registrations, 'other-server');

    expect(failing.showConnectionLostIcon).toBe(true);
  });

  it('clears a failed inactive catch-up on wake and catches up again at once', async () => {
    const active = new FakeServerConnection();
    const inactive = new FakeServerConnection();
    inactive.realtimeUrl = 'ws://wake.test/api/realtime';

    eventBusManager.synchronizeAuthenticatedServers(
      [
        {
          serverId: 'wake-active',
          connection: active as unknown as ServerConnection,
          projectionSupported: true,
          sync: new RealtimeProjectionSyncState(),
          projectionHandler: vi.fn()
        },
        {
          serverId: 'wake-inactive',
          connection: inactive as unknown as ServerConnection,
          projectionSupported: true,
          sync: new RealtimeProjectionSyncState(),
          projectionHandler: vi.fn()
        }
      ],
      'wake-active'
    );

    // The catch-up fails while the device sleeps.
    sockets[1].serverClose();
    expect(inactive.status).toBe('disconnected');
    await flushPromises();

    inactive.forceReconnect('tab visible after 600s hidden');
    expect(inactive.status).toBe('dormant');
    await flushPromises();
    expect(sockets).toHaveLength(3);
    expect(sockets[2].url).toBe(inactive.realtimeUrl);

    // A second wake signal replaces the catch-up in flight without a failure.
    inactive.forceReconnect('network came back online');
    expect(sockets[2].closeCalls.at(-1)?.reason).toBe('dormant');
    await flushPromises();
    await flushPromises();
    expect(sockets).toHaveLength(4);
    expect(inactive.status).toBe('dormant');

    sockets[3].open();
    await sockets[3].receive(
      serverFrame({ case: 'caughtUp', value: new RealtimeCaughtUp({ cursor: 'woken' }) })
    );
    expect(inactive.statusUpdates.at(-1)).toBe('dormant');
    expect(inactive.statusUpdates.filter((status) => status === 'disconnected')).toHaveLength(1);
  });

  it('skips a periodic poll that arrives while a slow cycle is still running', async () => {
    vi.useFakeTimers();
    setRealtimePollRandomForTests(() => 0.5);
    const connections = ['slow-a', 'slow-b', 'slow-c'].map((serverId) => ({
      serverId,
      connection: new FakeServerConnection() as unknown as ServerConnection,
      projectionSupported: true,
      sync: new RealtimeProjectionSyncState(),
      projectionHandler: vi.fn()
    }));

    eventBusManager.synchronizeAuthenticatedServers(connections, null);

    // Each unanswered catch-up times out after 30 seconds, so this cycle is
    // still running when the periodic poll fires at 60 seconds.
    expect(sockets).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(100_000);
    expect(sockets).toHaveLength(3);
    await vi.advanceTimersByTimeAsync(20_000);
    expect(sockets).toHaveLength(4);
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
          sync: new RealtimeProjectionSyncState(),
          projectionHandler: vi.fn()
        },
        {
          serverId: 'periodic-inactive',
          connection: inactive as unknown as ServerConnection,
          projectionSupported: true,
          sync: inactiveSync,
          projectionHandler: vi.fn()
        }
      ],
      'periodic-active'
    );

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
