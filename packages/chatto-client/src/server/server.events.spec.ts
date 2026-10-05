// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { CreateMessageRequest, CreateMessageResponse } from '@chatto/api-types/api/v1/messages_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { ConnectAPIConfig } from '../api/connect.js';
import type { CurrentUser } from '../api/viewer.js';
import type { RealtimeStatus } from '../messaging/types.js';

const mocks = vi.hoisted(() => ({
  viewer: vi.fn<(config: ConnectAPIConfig) => Promise<CurrentUser>>()
}));
vi.mock('../api/server.js', async (original) => ({
  ...(await original<typeof import('../api/server.js')>()),
  getPublicServerInfo: vi.fn(async () => ({ name: 'Chat', version: '0.5.0' }))
}));
vi.mock('../api/viewer.js', async (original) => ({
  ...(await original<typeof import('../api/viewer.js')>()),
  getCurrentUserViaConnect: mocks.viewer
}));

import { createClient, type ChattoClient } from '../client.js';
import { RealtimeProjectionUpdate } from '../realtime/eventBus.js';
import { setRealtimeSocketFactoryForTests } from './realtimeTransport.js';
import { inertRealtimeSocket } from '../testing/inertSocket.js';

function dmEvent(id = 'incoming', body = 'hello') {
  return new RealtimeEvent({
    id,
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: {
        roomId: 'room',
        roomKind: RoomKind.DM,
        bodyPlaintext: body,
        threadRootEventId: 'root'
      }
    }
  });
}

/** Requests that the stubbed server received, as `Service/Method` and body. */
let received: { method: string; body: Uint8Array }[] = [];
let client: ChattoClient;

beforeEach(() => {
  received = [];
  mocks.viewer.mockReset().mockResolvedValue({ id: 'bot', login: 'bot' } as CurrentUser);
  setRealtimeSocketFactoryForTests(inertRealtimeSocket);
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      const method = new URL(url).pathname.split('.').pop()!;
      received.push({ method, body: new Uint8Array(init.body as ArrayBuffer) });
      const body =
        method === 'MessageService/CreateMessage'
          ? new Uint8Array(new CreateMessageResponse({ message: { id: 'reply' } }).toBinary())
          : new Uint8Array();
      return new Response(body, { headers: { 'Content-Type': 'application/proto' } });
    })
  );
  client = createClient();
});

afterEach(() => {
  client.close();
  setRealtimeSocketFactoryForTests(null);
  vi.unstubAllGlobals();
});

/** A ready connection with controls for its realtime stream. */
async function connection() {
  const connection = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
  await connection.ready();
  await vi.waitFor(() => expect(client.realtime.getBus(connection.serverId)).toBeDefined());
  const bus = client.realtime.getBus(connection.serverId)!;
  return {
    connection,
    emit: (event: RealtimeEvent) => bus.publish(new RealtimeProjectionUpdate({ event })),
    reset: () => bus.publish(new RealtimeProjectionUpdate({ reset: true })),
    setStatus: (status: 'connected' | 'connecting' | 'disconnected') =>
      connection.connection.setRealtimeConnectionStatus(status)
  };
}

describe('consumeEvents', () => {
  it('handles events in order, reports status and gaps, and stops on abort', async () => {
    const { connection: chatto, emit, reset, setStatus } = await connection();
    const controller = new AbortController();
    const handled: string[] = [];
    const statuses: RealtimeStatus[] = [];
    let release!: () => void;
    const consuming = chatto.consumeEvents({
      signal: controller.signal,
      onStatus: (status) => statuses.push(status),
      async onEvent(event) {
        handled.push(event.id);
        if (event.id === 'first') await new Promise<void>((resolve) => (release = resolve));
      }
    });
    reset(); // the initial snapshot
    setStatus('connected');
    emit(dmEvent('first'));
    emit(dmEvent('second'));
    await vi.waitFor(() => expect(handled).toEqual(['first']));
    release();
    await vi.waitFor(() => expect(handled).toEqual(['first', 'second']));
    // A replacement snapshot after a connected stream is a gap. It is
    // reported with the next `ready`, not while the snapshot loads.
    reset();
    expect(statuses.at(-1)).toEqual({ state: 'ready', gap: false });
    setStatus('connecting');
    setStatus('connected');
    expect(statuses).toEqual([
      { state: 'connecting' },
      { state: 'ready', gap: false },
      { state: 'reconnecting' },
      { state: 'ready', gap: true }
    ]);
    controller.abort();
    await consuming;
  });

  it('resolves at once, without a status, for a signal that already aborted', async () => {
    const { connection: chatto } = await connection();
    const onStatus = vi.fn();
    await expect(
      chatto.consumeEvents({ signal: AbortSignal.abort(), onStatus, onEvent: () => {} })
    ).resolves.toBeUndefined();
    expect(onStatus).not.toHaveBeenCalled();
  });

  it('resolves when the connection closes and reports no reconnect', async () => {
    const { connection: chatto } = await connection();
    const statuses: string[] = [];
    const consuming = chatto.consumeEvents({
      onStatus: ({ state }) => statuses.push(state),
      onEvent: () => {}
    });
    chatto.close();
    await expect(consuming).resolves.toBeUndefined();
    expect(statuses).toEqual(['connecting']);
  });

  it.each(['session', 'protocol'] as const)(
    'rejects when the connection cannot deliver events: %s',
    async (end) => {
      const { connection: chatto } = await connection();
      const statuses: string[] = [];
      const consuming = chatto.consumeEvents({
        onStatus: ({ state }) => statuses.push(state),
        onEvent: () => {}
      });
      if (end === 'session') client.registry.handleAuthenticationRequired(chatto.serverId);
      else chatto.connection.markRealtimeUnsupported();
      await expect(consuming).rejects.toThrow(
        end === 'session' ? 'ended the session' : 'realtime protocol'
      );
      expect(statuses).toEqual(['connecting']);
    }
  );

  it('reports no reconnect for a session that ends right after the stream closed', async () => {
    const { connection: chatto, setStatus } = await connection();
    setStatus('connected');
    const statuses: string[] = [];
    const consuming = chatto.consumeEvents({
      onStatus: ({ state }) => statuses.push(state),
      onEvent: () => {}
    });
    // As the transport does: session handlers queue the end, then the stream
    // reports that it is disconnected.
    queueMicrotask(() => client.registry.handleAuthenticationRequired(chatto.serverId));
    setStatus('disconnected');
    await expect(consuming).rejects.toThrow('ended the session');
    expect(statuses).toEqual(['ready']);
  });

  it('drops the backlog and reports a gap behind a slow handler', async () => {
    const { connection: chatto, emit, setStatus } = await connection();
    const controller = new AbortController();
    const handled: string[] = [];
    const statuses: RealtimeStatus[] = [];
    let release!: () => void;
    const consuming = chatto.consumeEvents({
      signal: controller.signal,
      onStatus: (status) => statuses.push(status),
      async onEvent(event) {
        handled.push(event.id);
        if (event.id === 'event-0') await new Promise<void>((resolve) => (release = resolve));
      }
    });
    setStatus('connected');
    emit(dmEvent('event-0'));
    await vi.waitFor(() => expect(handled).toEqual(['event-0']));
    for (let index = 1; index < 1003; index++) emit(dmEvent(`event-${index}`));
    expect(statuses).toContainEqual({ state: 'ready', gap: true });
    release();
    await vi.waitFor(() => expect(handled.at(-1)).toBe('event-1002'));
    expect(handled).toHaveLength(3);
    controller.abort();
    await consuming;
  });

  it('rejects with the error of a failing status or event handler', async () => {
    const { connection: chatto, emit, setStatus } = await connection();
    const failingStatus = chatto.consumeEvents({
      onStatus: () => {
        throw new Error('status handler failed');
      },
      onEvent: () => {}
    });
    expect(() => setStatus('connected')).not.toThrow();
    await expect(failingStatus).rejects.toThrow('status handler failed');

    const controller = new AbortController();
    const removed = vi.spyOn(controller.signal, 'removeEventListener');
    const failingEvent = chatto.consumeEvents({
      signal: controller.signal,
      onEvent: () => Promise.reject(new Error('handler failed'))
    });
    emit(dmEvent());
    await expect(failingEvent).rejects.toThrow('handler failed');
    expect(removed).toHaveBeenCalledWith('abort', expect.any(Function));
  });

  it('keeps events from before ready() for the first consumer and reports a gap to a later one', async () => {
    let identify!: (user: CurrentUser) => void;
    mocks.viewer.mockReturnValueOnce(new Promise((resolve) => (identify = resolve)));
    const chatto = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    await vi.waitFor(() => expect(client.realtime.getBus(chatto.serverId)).toBeDefined());
    const bus = client.realtime.getBus(chatto.serverId)!;
    bus.publish(new RealtimeProjectionUpdate({ event: dmEvent('before-ready') }));
    identify({ id: 'bot', login: 'bot' } as CurrentUser);
    await chatto.ready();
    chatto.connection.setRealtimeConnectionStatus('connected');

    const handled: string[] = [];
    const statuses: RealtimeStatus[] = [];
    for (const expectedGap of [false, true]) {
      const controller = new AbortController();
      const consuming = chatto.consumeEvents({
        signal: controller.signal,
        onStatus: (status) => statuses.push(status),
        onEvent: (event) => {
          handled.push(event.id);
        }
      });
      await vi.waitFor(() => expect(statuses.at(-1)).toEqual({ state: 'ready', gap: expectedGap }));
      controller.abort();
      await consuming;
    }
    expect(handled).toEqual(['before-ready']);
  });
});

describe('run startup', () => {
  it('waits through an unreachable server until the viewer loads', async () => {
    vi.useFakeTimers();
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      mocks.viewer.mockRejectedValueOnce(new TypeError('fetch failed'));
      const chatto = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
      let settled = false;
      const statuses: RealtimeStatus[] = [];
      const running = chatto
        .run(() => {}, { onStatus: (status) => statuses.push(status) })
        .finally(() => (settled = true));
      await vi.advanceTimersByTimeAsync(30_000);
      expect(settled).toBe(false);
      // Startup reports each failed attempt, and never the same plain status twice in a row.
      expect(statuses[0]).toEqual({ state: 'connecting' });
      expect(statuses[1]).toMatchObject({
        state: 'connecting',
        error: expect.objectContaining({ message: expect.stringContaining('viewer') })
      });
      const plain = statuses.map((status) =>
        status.state !== 'ready' && !status.error ? status.state : JSON.stringify(status)
      );
      expect(plain.some((status, index) => index > 0 && status === plain[index - 1])).toBe(false);
      expect(chatto.viewerId).toBe('bot');
      chatto.close();
      await vi.advanceTimersByTimeAsync(0);
      await expect(running).resolves.toBeUndefined();
    } finally {
      logged.mockRestore();
      vi.useRealTimers();
    }
  });

  it('resolves when the connection closes before the viewer loads', async () => {
    mocks.viewer.mockReturnValueOnce(new Promise(() => {}));
    const chatto = client.connect({ serverUrl: 'https://chat.example', apiKey: 'key' });
    const running = chatto.run(() => {});
    chatto.close();
    await expect(running).resolves.toBeUndefined();
  });

  it('rejects when the server rejects the key', async () => {
    mocks.viewer.mockImplementation(async () => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });
    const chatto = client.connect({ serverUrl: 'https://chat.example', apiKey: 'revoked' });
    await expect(chatto.run(() => {})).rejects.toThrow('rejected the API key');
  });
});

describe('run', () => {
  it('aborts a running handler when the server ends the session', async () => {
    const { connection: chatto, emit } = await connection();
    let aborted: Promise<unknown> | undefined;
    const running = chatto.run((ctx) => {
      aborted = new Promise((resolve) =>
        ctx.signal.addEventListener('abort', () => resolve(ctx.signal.reason), { once: true })
      );
      return aborted.then(() => undefined);
    });
    emit(dmEvent());
    await vi.waitFor(() => expect(aborted).toBeDefined());
    client.registry.handleAuthenticationRequired(chatto.serverId);
    await expect(aborted).resolves.toMatchObject({ message: expect.stringContaining('ended') });
    await expect(running).rejects.toThrow('ended the session');
  });

  it('answers addressed messages through the context and skips the rest', async () => {
    const { connection: chatto, emit } = await connection();
    const contexts: { key: string; body: string }[] = [];
    const running = chatto.run(async (ctx) => {
      contexts.push({ key: ctx.conversationKey, body: ctx.message.body });
      expect(ctx.viewerId).toBe('bot');
      expect(ctx.server).toBe(chatto);
      await expect(ctx.reply(`You said: ${ctx.message.body}`)).resolves.toEqual({
        ids: ['reply']
      });
    });
    emit(new RealtimeEvent({ id: 'own', actorId: 'bot', event: dmEvent().event }));
    emit(dmEvent('incoming', 'hello'));
    await vi.waitFor(() => expect(contexts).toHaveLength(1));
    await vi.waitFor(() =>
      expect(received.map(({ method }) => method)).toContain('MessageService/CreateMessage')
    );
    const created = CreateMessageRequest.fromBinary(
      received.find(({ method }) => method === 'MessageService/CreateMessage')!.body
    );
    expect(created).toMatchObject({
      roomId: 'room',
      threadRootEventId: 'root',
      inReplyTo: 'incoming',
      body: 'You said: hello'
    });
    expect(contexts[0]!.key).toBe(JSON.stringify(['bot', 'room', 'root', 'human']));
    chatto.close();
    await expect(running).resolves.toBeUndefined();
  });

  it('aborts the context signal when the connection closes', async () => {
    const { connection: chatto, emit } = await connection();
    let aborted: Promise<void> | undefined;
    const running = chatto.run((ctx) => {
      aborted = new Promise((resolve) =>
        ctx.signal.addEventListener('abort', () => resolve(), { once: true })
      );
      return aborted;
    });
    emit(dmEvent());
    await vi.waitFor(() => expect(aborted).toBeDefined());
    chatto.close();
    await aborted;
    await expect(running).resolves.toBeUndefined();
  });

  it('reports handler failures to onError and keeps running, or rejects without it', async () => {
    const { connection: chatto, emit } = await connection();
    const errors: string[] = [];
    const controller = new AbortController();
    const handled: string[] = [];
    const running = chatto.run(
      (ctx) => {
        handled.push(ctx.message.id);
        if (ctx.message.id === 'first') throw new Error('handler failed');
      },
      {
        signal: controller.signal,
        onError: (error, event) => errors.push(`${event.id}: ${(error as Error).message}`)
      }
    );
    emit(dmEvent('first'));
    emit(dmEvent('second'));
    await vi.waitFor(() => expect(handled).toEqual(['first', 'second']));
    expect(errors).toEqual(['first: handler failed']);
    controller.abort();
    await running;

    const strict = chatto.run(() => {
      throw new Error('strict failure');
    });
    // The new loop subscribes after ready() resolves.
    await new Promise((resolve) => setTimeout(resolve, 0));
    emit(dmEvent('third'));
    await expect(strict).rejects.toThrow('strict failure');
  });
});

describe('request helpers', () => {
  it('send the server and context requests to their services', async () => {
    const { connection: chatto, emit } = await connection();
    const destination = { roomId: 'room', threadRootId: 'root' };
    const methods = () => received.map(({ method }) => method);

    await expect(chatto.createMessage(destination, 'hi')).resolves.toEqual({ id: 'reply' });
    await expect(chatto.postMessage(destination, 'hi')).resolves.toEqual({ ids: ['reply'] });
    await chatto.addReaction({ roomId: 'room', messageId: 'm1' }, '👍');
    await chatto.refreshTyping(destination);
    await expect(chatto.withTyping(destination, async () => 'done')).resolves.toBe('done');
    await expect(chatto.getMessage({ roomId: 'room', messageId: 'm1' })).resolves.toBeUndefined();
    await expect(chatto.readThread(destination)).rejects.toThrow('did not return the thread page');
    await expect(chatto.addressedMessage(dmEvent('direct'))).resolves.toMatchObject({
      id: 'direct',
      body: 'hello'
    });
    expect(methods()).toEqual([
      'MessageService/CreateMessage',
      'MessageService/CreateMessage',
      'MessageService/AddReaction',
      'RoomService/RefreshTypingIndicator',
      'RoomService/RefreshTypingIndicator',
      'MessageService/GetMessage',
      'ThreadService/GetThreadEvents'
    ]);

    received = [];
    const handled = new Promise<void>((resolve, reject) => {
      void chatto
        .run(async (ctx) => {
          await ctx.addReaction('👀');
          await ctx.refreshTyping();
          await ctx.withTyping(async () => undefined);
          await expect(ctx.readThread()).rejects.toThrow('did not return the thread page');
          resolve();
        })
        .catch(reject);
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    emit(dmEvent('incoming'));
    await handled;
    expect(methods()).toEqual([
      'MessageService/AddReaction',
      'RoomService/RefreshTypingIndicator',
      'RoomService/RefreshTypingIndicator',
      'ThreadService/GetThreadEvents'
    ]);
  });
});
