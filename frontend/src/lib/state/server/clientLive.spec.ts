import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ClientLiveClientFrameSchema,
  ClientLiveErrorSchema,
  ClientLiveHelloSchema,
  ClientLiveResponseSchema,
  ClientLiveServerFrameSchema
} from '$lib/pb/chatto/core/v1/client_live_pb';
import { AssetDeletedEventSchema } from '$lib/pb/chatto/core/v1/asset_events_pb';
import { LiveRoomEventSchema } from '$lib/pb/chatto/core/v1/live_events_pb';
import { liveRoomEventToEnvelope, startClientLiveSubscription } from './clientLive';
import type { LiveInfo, RegisteredServer } from './registry.svelte';

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: MockWebSocket[] = [];

  binaryType: BinaryType = 'blob';
  readyState = MockWebSocket.CONNECTING;
  sent: Uint8Array[] = [];
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<ArrayBuffer>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(readonly url: string) {
    MockWebSocket.instances.push(this);
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
    if (data instanceof Uint8Array) {
      this.sent.push(data);
      return;
    }
    if (data instanceof ArrayBuffer) {
      this.sent.push(new Uint8Array(data));
      return;
    }
    throw new Error('unexpected websocket test payload');
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({} as CloseEvent);
  }

  open(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.({} as Event);
  }

  receive(frame: Parameters<typeof toBinary<typeof ClientLiveServerFrameSchema>>[1]): void {
    const bytes = toBinary(ClientLiveServerFrameSchema, frame);
    const data = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
    this.onmessage?.({ data } as MessageEvent<ArrayBuffer>);
  }
}

const server: RegisteredServer = {
  id: 'server-1',
  url: 'https://chat.example.test',
  name: 'Test',
  iconUrl: null,
  token: 'bearer-token',
  userId: 'U1',
  userLogin: 'u1',
  userDisplayName: 'User One',
  userAvatarUrl: null,
  live: null,
  addedAt: 1
};

const info: LiveInfo = {
  url: 'wss://chat.example.test/api/live',
  tokenUrl: '/api/live-token',
  protocol: 'chatto.client-live-protobuf.v1'
};

function start() {
  return startClientLiveSubscription({
    server,
    info,
    onEvent: vi.fn(),
    onCatchUpNeeded: vi.fn(),
    onEnd: vi.fn(),
    onError: vi.fn()
  });
}

function serverHello(capabilities = [
  'live.events.v1',
  'live.requests.v1',
  'history.room_events.v1',
  'history.thread_events.v1'
]) {
  return create(ClientLiveServerFrameSchema, {
    payload: {
      case: 'hello',
      value: create(ClientLiveHelloSchema, {
        protocol: info.protocol,
        capabilities
      })
    }
  });
}

describe('client live websocket request mux', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        json: async () => ({ ticket: 'ticket-1', url: 'wss://chat.example.test/api/live' })
      }))
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('waits for the socket to open, sends a typed request frame, and resolves the matching response', async () => {
    const subscription = start();
    await vi.waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
    const ws = MockWebSocket.instances[0];

    const responsePromise = subscription.request('room.events', new Uint8Array([1, 2, 3]));
    expect(ws.sent).toHaveLength(0);

    ws.open();
    ws.receive(serverHello());
    await vi.waitFor(() => expect(ws.sent).toHaveLength(1));

    const requestFrame = fromBinary(ClientLiveClientFrameSchema, ws.sent[0]);
    expect(requestFrame.requestId).toBe(1n);
    expect(requestFrame.payload.case).toBe('request');
    if (requestFrame.payload.case !== 'request') {
      throw new Error('expected request payload');
    }
    expect(requestFrame.payload.value.type).toBe('room.events');
    expect(Array.from(requestFrame.payload.value.payload)).toEqual([1, 2, 3]);

    ws.receive(
      create(ClientLiveServerFrameSchema, {
        requestId: 1n,
        payload: {
          case: 'response',
          value: create(ClientLiveResponseSchema, {
            type: 'room.events',
            payload: new Uint8Array([9, 8, 7])
          })
        }
      })
    );

    await expect(responsePromise).resolves.toEqual(new Uint8Array([9, 8, 7]));
  });

  it('rejects only the matching pending request when the server returns a request error', async () => {
    const subscription = start();
    await vi.waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
    const ws = MockWebSocket.instances[0];
    ws.open();
    ws.receive(serverHello());

    const first = subscription.request('room.events', new Uint8Array([1]));
    const second = subscription.request('room.event', new Uint8Array([2]));
    await vi.waitFor(() => expect(ws.sent).toHaveLength(2));

    ws.receive(
      create(ClientLiveServerFrameSchema, {
        requestId: 1n,
        payload: {
          case: 'error',
          value: create(ClientLiveErrorSchema, {
            code: 'forbidden',
            message: 'not a member of this room'
          })
        }
      })
    );
    ws.receive(
      create(ClientLiveServerFrameSchema, {
        requestId: 2n,
        payload: {
          case: 'response',
          value: create(ClientLiveResponseSchema, {
            type: 'room.event',
            payload: new Uint8Array([4])
          })
        }
      })
    );

    await expect(first).rejects.toThrow('not a member of this room');
    await expect(second).resolves.toEqual(new Uint8Array([4]));
  });

  it('uses same-origin cookie auth for the live ticket when no bearer token is registered', async () => {
    const origin = 'https://local.chatto.test';
    vi.stubGlobal('window', {
      location: {
        origin,
        href: `${origin}/chat`
      }
    });
    const sameOriginServer = {
      ...server,
      url: origin,
      token: null
    };
    startClientLiveSubscription({
      server: sameOriginServer,
      info: { ...info, url: '/api/live' },
      onEvent: vi.fn(),
      onCatchUpNeeded: vi.fn(),
      onEnd: vi.fn(),
      onError: vi.fn()
    });

    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    expect(fetch).toHaveBeenCalledWith(
      `${origin}/api/live-token`,
      expect.objectContaining({
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' }
      })
    );
  });

  it('does not send requests until the server hello advertises required capabilities', async () => {
    const onError = vi.fn();
    const subscription = startClientLiveSubscription({
      server,
      info,
      onEvent: vi.fn(),
      onCatchUpNeeded: vi.fn(),
      onEnd: vi.fn(),
      onError
    });
    await vi.waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
    const ws = MockWebSocket.instances[0];

    const responsePromise = subscription.request('room.events', new Uint8Array([1]));
    ws.open();
    await Promise.resolve();
    expect(ws.sent).toHaveLength(0);

    ws.receive(serverHello(['live.events.v1']));

    await expect(responsePromise).rejects.toThrow('missing capabilities');
    expect(onError).toHaveBeenCalled();
  });

  it('rejects the live ticket when the token endpoint reports a mismatched protocol', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        ticket: 'ticket-1',
        url: 'wss://chat.example.test/api/live',
        protocol: 'wrong.protocol'
      })
    } as Response);

    const onError = vi.fn();
    startClientLiveSubscription({
      server,
      info,
      onEvent: vi.fn(),
      onCatchUpNeeded: vi.fn(),
      onEnd: vi.fn(),
      onError
    });

    await vi.waitFor(() => expect(onError).toHaveBeenCalled());
    expect(String(onError.mock.calls[0][0])).toContain('wrong.protocol');
    expect(MockWebSocket.instances).toHaveLength(0);
  });
});

describe('client live room event adapter', () => {
  it('keeps room scope on asset deletions so room-scoped stores can ingest them', () => {
    const envelope = liveRoomEventToEnvelope(
      create(LiveRoomEventSchema, {
        id: 'E-delete',
        roomId: 'R1',
        event: {
          case: 'assetDeleted',
          value: create(AssetDeletedEventSchema, { assetId: 'A1' })
        }
      })
    );

    expect(envelope?.event).toMatchObject({
      __typename: 'AssetDeletedEvent',
      assetId: 'A1',
      deletedRoomId: 'R1'
    });
  });
});
