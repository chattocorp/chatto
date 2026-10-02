// @vitest-environment node
import { describe, expect, test, vi } from 'vitest';
import {
  Code,
  ConnectError,
  createClient,
  createRouterTransport,
  type ConnectRouter
} from '@connectrpc/connect';
import { AssetService } from '@chatto/api-types/api/v1/attachments_connect';
import type { BatchGetAssetsRequest } from '@chatto/api-types/api/v1/attachments_pb';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { CreateMessageRequest } from '@chatto/api-types/api/v1/messages_pb';
import type { GetThreadEventsRequest } from '@chatto/api-types/api/v1/room_timeline_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import {
  conversationKey,
  MessagingRequests,
  replyDestination,
  type AssetAccess
} from './requests.js';
import { createDeliveryTracker } from './deliveries.js';

/** Request helpers for the viewer `bot`, answered by an in-memory Connect router. */
function requests(routes: (router: ConnectRouter) => void = () => {}, assets?: AssetAccess) {
  const transport = createRouterTransport(routes);
  const viewerId = vi.fn(async () => 'bot');
  return {
    viewerId,
    requests: new MessagingRequests(
      { service: (service) => createClient(service, transport) },
      viewerId,
      assets
    )
  };
}

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

describe('messages', () => {
  test('replies in the thread of the prompting message and sends nothing after cancellation', async () => {
    const created: CreateMessageRequest[] = [];
    const { requests: api } = requests((router) =>
      router.service(MessageService, {
        createMessage(request) {
          created.push(request);
          return { message: { id: 'reply' } };
        }
      })
    );
    const message = (await api.addressedMessage(dmEvent()))!;
    expect(message.reasons).toEqual(['direct_message']);
    await expect(api.reply(message, 'hi')).resolves.toEqual({ ids: ['reply'] });
    expect(created[0]).toMatchObject({
      roomId: 'room',
      threadRootEventId: 'root',
      inReplyTo: 'incoming',
      body: 'hi'
    });
    await expect(
      api.reply(message, 'no', { signal: AbortSignal.abort(new Error('stop')) })
    ).rejects.toThrow();
    expect(created).toHaveLength(1);
  });

  test('splits long messages at 8000 code points and sends them in order', async () => {
    const bodies: string[] = [];
    const { requests: api } = requests((router) =>
      router.service(MessageService, {
        createMessage(request) {
          bodies.push(request.body);
          return { message: { id: `part-${bodies.length}` } };
        }
      })
    );
    await expect(
      api.postMessage({ roomId: 'room', threadRootId: 'root' }, '😀'.repeat(8001))
    ).resolves.toEqual({ ids: ['part-1', 'part-2'] });
    expect(bodies.map((body) => Array.from(body).length)).toEqual([8000, 1]);
    await api.postMessage({ roomId: 'room', threadRootId: 'root' }, '');
    expect(bodies).toHaveLength(3);
  });

  test('reports a message without text without a body', async () => {
    const { requests: api } = requests((router) =>
      router.service(MessageService, {
        getMessage: ({ eventId }) => ({
          message: {
            id: eventId,
            roomId: 'room',
            actorId: 'alice',
            body: eventId === 'text' ? 'hi' : ''
          }
        })
      })
    );
    await expect(api.getMessage({ roomId: 'room', messageId: 'files' })).resolves.toEqual({
      id: 'files',
      roomId: 'room',
      authorId: 'alice'
    });
    await expect(api.getMessage({ roomId: 'room', messageId: 'text' })).resolves.toMatchObject({
      body: 'hi'
    });
  });

  test('adds a reaction to the given message', async () => {
    const reactions: unknown[] = [];
    const { requests: api } = requests((router) =>
      router.service(MessageService, {
        addReaction(request) {
          reactions.push({ ...request });
          return {};
        }
      })
    );
    await api.addReaction({ roomId: 'room', messageId: 'm1' }, 'eyes');
    expect(reactions).toEqual([
      expect.objectContaining({ roomId: 'room', messageEventId: 'm1', emoji: 'eyes' })
    ]);
  });
});

describe('threads', () => {
  const posted = (id: string, actorId: string, body: string) => ({
    id,
    actorId,
    event: { case: 'messagePosted' as const, value: { message: { id, actorId, body } } }
  });

  test('reads the root and newest replies with names, then only newer messages', async () => {
    const reads: GetThreadEventsRequest[] = [];
    const { requests: api } = requests((router) =>
      router.service(ThreadService, {
        getThreadEvents(request) {
          reads.push(request);
          if (request.cursor.case !== 'after')
            return {
              page: {
                events: [posted('root', 'human', 'question'), posted('one', 'bot', 'answer')],
                hasOlder: true,
                endCursor: 'c1',
                includes: {
                  users: {
                    human: { login: 'alice', displayName: 'Alice Doe', bio: 'private' },
                    bot: { login: 'chatto_bot' }
                  }
                }
              }
            };
          return request.cursor.value === 'c1'
            ? {
                page: {
                  events: [posted('two', 'human', 'thanks')],
                  hasNewer: true,
                  endCursor: 'c2'
                }
              }
            : {
                page: {
                  events: [posted('three', 'human', 'bye')],
                  hasNewer: false,
                  endCursor: 'c3'
                }
              };
        }
      })
    );
    const location = { roomId: 'room', threadRootId: 'root' };
    const first = await api.readThread(location);
    expect(first).toEqual({
      messages: [
        {
          id: 'root',
          authorId: 'human',
          authorName: 'Alice Doe',
          authorLogin: 'alice',
          body: 'question',
          fromViewer: false
        },
        {
          id: 'one',
          authorId: 'bot',
          authorName: 'chatto_bot',
          authorLogin: 'chatto_bot',
          body: 'answer',
          fromViewer: true
        }
      ],
      cursor: 'c1',
      olderOmitted: true
    });
    expect(JSON.stringify(first)).not.toContain('private');
    const next = await api.readThread(location, { after: first.cursor });
    expect(next.messages.map(({ id, fromViewer }) => [id, fromViewer])).toEqual([
      ['two', false],
      ['three', false]
    ]);
    expect(next).toMatchObject({ cursor: 'c3', olderOmitted: false });
    expect(reads.map((request) => request.cursor.value)).toEqual([undefined, 'c1', 'c2']);
    expect(reads[0]?.limit).toBe(100);
  });

  test('rejects an empty cursor before any request', async () => {
    const { requests: api, viewerId } = requests();
    await expect(
      api.readThread({ roomId: 'room', threadRootId: 'root' }, { after: '' })
    ).rejects.toThrow('must not be empty');
    expect(viewerId).not.toHaveBeenCalled();
  });

  test('keeps the cursor of an up-to-date thread', async () => {
    const { requests: api } = requests((router) =>
      router.service(ThreadService, {
        getThreadEvents: () => ({ page: { events: [], hasNewer: false } })
      })
    );
    await expect(
      api.readThread({ roomId: 'room', threadRootId: 'root' }, { after: 'c3' })
    ).resolves.toEqual({ messages: [], cursor: 'c3', olderOmitted: false });
  });

  test('rejects pagination that does not advance', async () => {
    const { requests: api } = requests((router) =>
      router.service(ThreadService, {
        getThreadEvents: () => ({ page: { events: [], hasNewer: true, endCursor: 'same' } })
      })
    );
    await expect(
      api.readThread({ roomId: 'room', threadRootId: 'root' }, { after: 'same' })
    ).rejects.toThrow('did not advance');
  });
});

describe('attachments', () => {
  test('thread reads keep messages with attachments only and leave out deleted messages', async () => {
    const { requests: api } = requests((router) =>
      router.service(ThreadService, {
        getThreadEvents: () => ({
          page: {
            events: ['files', 'deleted'].map((id) => ({
              id,
              actorId: 'human',
              event: {
                case: 'messagePosted' as const,
                value: {
                  message: {
                    id,
                    actorId: 'human',
                    attachments: [
                      {
                        id: `${id}-a`,
                        filename: 'shot.png',
                        contentType: 'image/png',
                        width: 800,
                        height: 600,
                        assetUrl: { url: '/assets/files/a?ticket=secret' }
                      }
                    ],
                    ...(id === 'deleted' ? { deletedAt: { seconds: 1n } } : {})
                  }
                }
              }
            }))
          }
        })
      })
    );
    const read = await api.readThread({ roomId: 'room', threadRootId: 'root' });
    expect(read.messages).toEqual([
      {
        id: 'files',
        authorId: 'human',
        body: '',
        attachments: [
          { id: 'files-a', filename: 'shot.png', contentType: 'image/png', width: 800, height: 600 }
        ],
        fromViewer: false
      }
    ]);
    expect(JSON.stringify(read)).not.toContain('ticket');
  });

  /** Request helpers with one asset, `a`, and a fetch that answers its addresses. */
  function withAsset(
    asset: { contentType: string; size: number },
    respond: (url: URL) => Response = () => new Response('content')
  ) {
    const lookups: BatchGetAssetsRequest[] = [];
    const fetch = vi.fn<typeof globalThis.fetch>(async (input) => respond(new URL(String(input))));
    const { requests: api } = requests(
      (router) =>
        router.service(AssetService, {
          batchGetAssets(request) {
            lookups.push(request);
            return {
              assets: [
                {
                  id: 'a',
                  filename: 'file',
                  contentType: asset.contentType,
                  size: BigInt(asset.size),
                  assetUrl: { url: '/assets/files/a?t=1' },
                  thumbnailAssetUrl: { url: '/assets/files/a/image/1600x1600/contain?t=2' }
                }
              ]
            };
          }
        }),
      { origin: 'https://chat.example', fetch }
    );
    return { api, fetch, lookups };
  }

  test('reads a resized image from the thumbnail address without credentials', async () => {
    const { api, fetch, lookups } = withAsset(
      { contentType: 'image/png', size: 50_000_000 },
      () => new Response(new Uint8Array([1, 2, 3]), { headers: { 'content-type': 'image/webp' } })
    );
    const content = await api.readAttachment(
      { roomId: 'room', attachmentId: 'a' },
      { maxImageSize: 1600 }
    );
    expect(content).toEqual({
      filename: 'file',
      contentType: 'image/webp',
      data: new Uint8Array([1, 2, 3])
    });
    expect(lookups[0]?.thumbnail).toMatchObject({ width: 1600, height: 1600 });
    const [url, init] = fetch.mock.calls[0]!;
    expect(String(url)).toBe('https://chat.example/assets/files/a/image/1600x1600/contain?t=2');
    expect(init?.redirect).toBe('error');
    expect(init?.headers).toBeUndefined();
  });

  test('reads other files from the original address with download=1', async () => {
    const { api, fetch } = withAsset({ contentType: 'text/plain', size: 7 });
    const content = await api.readAttachment({ roomId: 'room', attachmentId: 'a' });
    expect(new TextDecoder().decode(content.data)).toBe('content');
    expect(String(fetch.mock.calls[0]![0])).toBe(
      'https://chat.example/assets/files/a?t=1&download=1'
    );
  });

  test('rejects content above the limit, declared or streamed', async () => {
    const declared = withAsset({ contentType: 'text/plain', size: 10_000 });
    await expect(
      declared.api.readAttachment({ roomId: 'room', attachmentId: 'a' }, { maxBytes: 100 })
    ).rejects.toThrow('too large');
    expect(declared.fetch).not.toHaveBeenCalled();
    const streamed = withAsset(
      { contentType: 'text/plain', size: 1 },
      () => new Response('x'.repeat(200))
    );
    await expect(
      streamed.api.readAttachment({ roomId: 'room', attachmentId: 'a' }, { maxBytes: 100 })
    ).rejects.toThrow('too large');
  });

  test('requests absolute addresses from its own origin', async () => {
    const { requests: api, fetch } = assetOnly('https://public.example/assets/files/a?t=1');
    await api.readAttachment({ roomId: 'room', attachmentId: 'a' });
    expect(String(fetch.mock.calls[0]![0])).toBe(
      'https://chat.example/assets/files/a?t=1&download=1'
    );
  });

  test('rejects unexpected addresses, failed downloads, and missing assets', async () => {
    const unexpected = assetOnly('https://evil.example/steal?t=1');
    await expect(
      unexpected.requests.readAttachment({ roomId: 'room', attachmentId: 'a' })
    ).rejects.toThrow('unexpected attachment address');
    expect(unexpected.fetch).not.toHaveBeenCalled();
    const failed = withAsset(
      { contentType: 'text/plain', size: 1 },
      () => new Response('no', { status: 403 })
    );
    await expect(failed.api.readAttachment({ roomId: 'room', attachmentId: 'a' })).rejects.toThrow(
      'HTTP 403'
    );
    await expect(
      failed.api.readAttachment({ roomId: 'room', attachmentId: 'missing' })
    ).rejects.toThrow('not found');
    await expect(
      requests().requests.readAttachment({ roomId: 'room', attachmentId: 'a' })
    ).rejects.toThrow('cannot read attachment content');
  });

  /** Request helpers whose text asset `a` has the original address `url`. */
  function assetOnly(url: string) {
    const fetch = vi.fn<typeof globalThis.fetch>(async () => new Response('x'));
    return {
      fetch,
      ...requests(
        (router) =>
          router.service(AssetService, {
            batchGetAssets: () => ({
              assets: [{ id: 'a', contentType: 'text/plain', assetUrl: { url } }]
            })
          }),
        { origin: 'https://chat.example', fetch }
      )
    };
  }
});

describe('addressing', () => {
  const event = () =>
    new RealtimeEvent({
      id: 'incoming',
      actorId: 'human',
      event: {
        case: 'messagePosted',
        value: {
          roomId: 'room',
          roomKind: RoomKind.CHANNEL,
          bodyPlaintext: 'hello',
          threadRootEventId: 'root',
          inReplyTo: 'target'
        }
      }
    });

  /** Request helpers whose message lookup returns `message`. */
  function lookup(message?: {
    id: string;
    roomId: string;
    actorId?: string;
    threadRootEventId?: string;
  }) {
    const getMessage = vi.fn(() => ({ message }));
    const { requests: api } = requests((router) => router.service(MessageService, { getMessage }));
    return { api, getMessage };
  }

  test('disabled reply recognition makes no lookup', async () => {
    const { api, getMessage } = lookup();
    expect(await api.addressedMessage(event(), { reasons: ['mention'] })).toBeUndefined();
    expect(getMessage).not.toHaveBeenCalled();
  });

  test('recognizes direct messages and mentions without a lookup and keeps empty text', async () => {
    const { api, getMessage } = lookup();
    const incoming = event();
    if (incoming.event.case !== 'messagePosted') throw new Error('fixture');
    incoming.event.value.roomKind = RoomKind.DM;
    incoming.event.value.bodyPlaintext = '';
    expect(await api.addressedMessage(incoming)).toMatchObject({
      id: 'incoming',
      authorId: 'human',
      body: '',
      roomId: 'room',
      threadRootId: 'root',
      reasons: ['direct_message']
    });
    incoming.event.value.roomKind = RoomKind.CHANNEL;
    incoming.event.value.mentions = [
      { includesViewer: true }
    ] as typeof incoming.event.value.mentions;
    expect((await api.addressedMessage(incoming))?.reasons).toEqual(['mention']);
    expect(getMessage).not.toHaveBeenCalled();
  });

  test.each(['self', 'missing-text', 'missing-id', 'missing-actor', 'other-event', 'unaddressed'])(
    'ignores %s without a lookup',
    async (kind) => {
      const { api, getMessage } = lookup();
      const incoming = event();
      if (incoming.event.case !== 'messagePosted') throw new Error('fixture');
      if (kind === 'self') incoming.actorId = 'bot';
      if (kind === 'missing-text') incoming.event.value.bodyPlaintext = undefined;
      if (kind === 'missing-id') incoming.id = '';
      if (kind === 'missing-actor') incoming.actorId = '';
      if (kind === 'unaddressed') incoming.event.value.inReplyTo = '';
      if (kind === 'other-event') incoming.event = { case: undefined };
      expect(await api.addressedMessage(incoming)).toBeUndefined();
      expect(getMessage).not.toHaveBeenCalled();
    }
  );

  test.each(['valid', 'wrong-author', 'wrong-room', 'wrong-id', 'wrong-thread', 'missing'])(
    'verifies the replied-to message: %s',
    async (kind) => {
      const { api, getMessage } = lookup(
        kind === 'missing'
          ? undefined
          : {
              id: kind === 'wrong-id' ? 'different' : 'target',
              roomId: kind === 'wrong-room' ? 'different' : 'room',
              actorId: kind === 'wrong-author' ? 'human' : 'bot',
              threadRootEventId: kind === 'wrong-thread' ? 'different' : 'root'
            }
      );
      const result = await api.addressedMessage(event());
      if (kind === 'valid') expect(result?.reasons).toEqual(['reply']);
      else expect(result).toBeUndefined();
      expect(getMessage).toHaveBeenCalledOnce();
    }
  );

  test('recognizes replies to a thread root that the viewer wrote', async () => {
    const { api } = lookup({ id: 'target', roomId: 'room', actorId: 'bot' });
    const incoming = event();
    if (incoming.event.case !== 'messagePosted') throw new Error('fixture');
    incoming.event.value.threadRootEventId = 'target';
    expect((await api.addressedMessage(incoming))?.reasons).toEqual(['reply']);
  });

  test('rejects lookup failures and cancellation instead of ignoring the message', async () => {
    const { api, getMessage } = lookup();
    getMessage.mockImplementationOnce(() => {
      throw new ConnectError('lookup failed', Code.Unavailable);
    });
    await expect(api.addressedMessage(event())).rejects.toThrow('lookup failed');
    await expect(
      api.addressedMessage(event(), { signal: AbortSignal.abort(new Error('cancelled')) })
    ).rejects.toThrow('cancelled');
    expect(getMessage).toHaveBeenCalledOnce();
  });
});

describe('conversations and deliveries', () => {
  test('default keys isolate viewer, room, thread, and sender; root messages reply in their own thread', () => {
    const message = { id: 'root', roomId: 'room', authorId: 'human' };
    const key = conversationKey('bot', message);
    expect(replyDestination(message)).toEqual({
      roomId: 'room',
      threadRootId: 'root',
      inReplyTo: 'root'
    });
    expect(conversationKey('bot', { ...message, threadRootId: 'root' })).toBe(key);
    expect(conversationKey('other', message)).not.toBe(key);
    for (const override of [
      { roomId: 'other' },
      { threadRootId: 'other' },
      { authorId: 'other' }
    ]) {
      expect(conversationKey('bot', { ...message, ...override })).not.toBe(key);
    }
  });

  test('deliveries remain retryable until explicitly accepted, then expire', async () => {
    let now = 0;
    const tracker = createDeliveryTracker({ retentionMs: 10, now: () => now });
    const accept = async (register: () => Promise<void>) => {
      if (tracker.has('message')) return;
      await register();
      tracker.accept('message');
    };
    await expect(
      accept(async () => {
        throw new Error('disk');
      })
    ).rejects.toThrow('disk');
    expect(tracker.has('message')).toBe(false);
    const register = vi.fn().mockResolvedValue(undefined);
    await accept(register);
    await accept(register);
    expect(register).toHaveBeenCalledOnce();
    expect(createDeliveryTracker().has('message')).toBe(false);
    now = 10;
    await accept(register);
    expect(register).toHaveBeenCalledTimes(2);
  });

  test.each([0, -1, Infinity, NaN])('rejects invalid delivery retention: %s', (retentionMs) => {
    expect(() => createDeliveryTracker({ retentionMs })).toThrow('positive and finite');
  });
});
