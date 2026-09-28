import { afterEach, expect, test, vi } from 'vitest';
import { createChattoClient, withTyping } from './index.js';

const destination = { roomId: 'room', threadRootId: 'root' };
const delivery = { roomId: 'room', threadRootId: 'root' };
const event = (id: string, body: string, actorId = 'human') => ({
  id,
  messagePosted: { message: { actorId, body } }
});
afterEach(() => vi.useRealTimers());

test('thread posts preserve the reply target on every chunk', async () => {
  const request = vi.fn<typeof fetch>().mockImplementation(async () => Response.json({}));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'key',
    fetch: request
  });
  await client.postMessage({ ...destination, inReplyTo: 'prompt' }, 'x'.repeat(8001));
  expect(request).toHaveBeenCalledTimes(2);
  for (const [, init] of request.mock.calls) {
    expect(JSON.parse(init!.body as string)).toMatchObject({
      threadRootEventId: 'root',
      inReplyTo: 'prompt'
    });
  }
});

test('pins RPCs to the configured server with bearer auth and no redirects', async () => {
  const request = vi.fn<typeof fetch>().mockResolvedValue(Response.json({}));
  const client = createChattoClient({
    serverUrl: 'https://chat.example/prefix',
    apiKey: 'secret',
    fetch: request
  });
  await client.refreshTyping(destination);
  expect(String(request.mock.calls[0]![0])).toBe(
    'https://chat.example/api/connect/chatto.api.v1.RoomService/RefreshTypingIndicator'
  );
  expect(request.mock.calls[0]![1]).toMatchObject({
    method: 'POST',
    redirect: 'error',
    headers: { Authorization: 'Bearer secret', 'Connect-Protocol-Version': '1' },
    body: JSON.stringify({ roomId: 'room', threadRootEventId: 'root' })
  });
  await expect(client.rpc('https://other.example/collect', {})).rejects.toThrow(
    'service and method'
  );
  expect(request).toHaveBeenCalledOnce();
});

test.each(['file:///tmp/chat', 'https://user:password@chat.example'])(
  'rejects unsafe server URL %s',
  (serverUrl) => {
    expect(() => createChattoClient({ serverUrl, apiKey: 'secret' })).toThrow(
      'without credentials'
    );
  }
);

test('does not disclose transport errors, response bodies, or retry uncertain writes', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockRejectedValueOnce(new Error('secret URL and token'))
    .mockResolvedValueOnce(new Response('private server error', { status: 503 }))
    .mockResolvedValueOnce(new Response('private malformed JSON'));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  await expect(client.createMessage(destination, 'private message')).rejects.toThrow(
    /^Chatto API request did not complete$/
  );
  await expect(client.createMessage(destination, 'private message')).rejects.toThrow(
    /^Chatto API returned HTTP 503$/
  );
  await expect(client.createMessage(destination, 'private message')).rejects.toThrow(
    /^Chatto API returned invalid JSON$/
  );
  expect(request).toHaveBeenCalledTimes(3);
});

test('preserves caller cancellation through the transport', async () => {
  vi.useFakeTimers();
  const request = vi.fn<typeof fetch>().mockImplementation(
    (_url, init) =>
      new Promise((_resolve, reject) => {
        init!.signal!.addEventListener('abort', () => reject(init!.signal!.reason), { once: true });
      })
  );
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  const controller = new AbortController();
  const cancelled = client.refreshTyping(destination, controller.signal);
  const assertion = expect(cancelled).rejects.toThrow('stop');
  controller.abort(new Error('stop'));
  await assertion;
  expect(request.mock.calls[0]![1]!.signal!.aborted).toBe(true);
});

test('does not send a request after cancellation', async () => {
  const request = vi.fn<typeof fetch>();
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  const controller = new AbortController();
  controller.abort(new Error('cancelled before delivery'));
  await expect(client.postMessage(destination, 'hello', controller.signal)).rejects.toThrow(
    'cancelled before delivery'
  );
  expect(request).not.toHaveBeenCalled();
});

test('splits Unicode without breaking code points and stops on the first failed chunk', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(Response.json({}))
    .mockResolvedValueOnce(new Response(null, { status: 403 }));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  await expect(client.postMessage(destination, '😀'.repeat(16001))).rejects.toThrow('403');
  expect(request).toHaveBeenCalledTimes(2);
  for (const [, init] of request.mock.calls) {
    expect(JSON.parse(String(init!.body)).body).toBe('😀'.repeat(8000));
  }
});

test('preserves reply targets and reacts to the triggering message', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockImplementation(async () => Response.json({ message: { id: 'reply' } }));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  expect(await client.createMessage(destination, 'hello', undefined, 'ping')).toEqual({
    message: { id: 'reply' }
  });
  expect(JSON.parse(String(request.mock.calls[0]![1]!.body))).toMatchObject({
    inReplyTo: 'ping',
    threadRootEventId: 'root'
  });
  await client.addReaction('room', 'ping', 'eyes');
  expect(JSON.parse(String(request.mock.calls[1]![1]!.body))).toEqual({
    roomId: 'room',
    messageEventId: 'ping',
    emoji: 'eyes'
  });
});

test('reads the root and the newest replies with author names, then only newer messages', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValueOnce(
      Response.json({
        page: {
          events: [event('root', 'hello'), event('two', 'again'), event('three', 'answer', 'bot')],
          hasOlder: true,
          endCursor: 'c3',
          includes: {
            users: {
              human: { login: 'alice', displayName: 'Alice Doe', bio: 'private' },
              bot: { login: 'chatto_bot' }
            }
          }
        }
      })
    )
    .mockResolvedValueOnce(
      Response.json({ page: { events: [event('four', 'more')], hasNewer: true, endCursor: 'c4' } })
    )
    .mockResolvedValueOnce(
      Response.json({ page: { events: [event('five', 'last')], hasNewer: false, endCursor: 'c5' } })
    );
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  const first = await client.readThread(delivery);
  expect(first).toEqual({
    messages: [
      {
        id: 'root',
        authorId: 'human',
        authorName: 'Alice Doe',
        authorLogin: 'alice',
        body: 'hello'
      },
      {
        id: 'two',
        authorId: 'human',
        authorName: 'Alice Doe',
        authorLogin: 'alice',
        body: 'again'
      },
      {
        id: 'three',
        authorId: 'bot',
        authorName: 'chatto_bot',
        authorLogin: 'chatto_bot',
        body: 'answer'
      }
    ],
    cursor: 'c3',
    olderOmitted: true
  });
  expect(JSON.parse(String(request.mock.calls[0]![1]!.body))).toEqual({
    roomId: 'room',
    threadRootEventId: 'root',
    limit: 100
  });
  const next = await client.readThread(delivery, undefined, { after: first.cursor });
  expect(next.messages.map((message) => message.id)).toEqual(['four', 'five']);
  expect(next.cursor).toBe('c5');
  expect(JSON.parse(String(request.mock.calls[1]![1]!.body))).toMatchObject({ after: 'c3' });
  expect(JSON.parse(String(request.mock.calls[2]![1]!.body))).toMatchObject({ after: 'c4' });
  expect(JSON.stringify(first)).not.toContain('private');
});

test('an up-to-date thread keeps its cursor', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockResolvedValue(Response.json({ page: { events: [], hasNewer: false } }));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'k',
    fetch: request
  });
  expect(await client.readThread(delivery, undefined, { after: 'c3' })).toEqual({
    messages: [],
    cursor: 'c3',
    olderOmitted: false
  });
});

test.each([
  [{}, undefined],
  [{ page: { hasNewer: true } }, 'c1'],
  [{ page: { hasNewer: true, endCursor: 'c1' } }, 'c1']
])('rejects missing or non-progressing pages %#', async (response, after) => {
  const request = vi.fn<typeof fetch>().mockImplementation(async () => Response.json(response));
  const client = createChattoClient({
    serverUrl: 'https://chat.example',
    apiKey: 'secret',
    fetch: request
  });
  await expect(client.readThread(delivery, undefined, after ? { after } : {})).rejects.toThrow(
    /thread page|pagination did not advance/
  );
  expect(request.mock.calls.length).toBeLessThanOrEqual(2);
});

test('typing stays best effort, never overlaps, and cancels when work finishes', async () => {
  vi.useFakeTimers();
  const pending = Promise.withResolvers<void>();
  const work = Promise.withResolvers<string>();
  let refreshSignal!: AbortSignal;
  const update = vi.fn(async (signal: AbortSignal) => {
    refreshSignal = signal;
    return pending.promise;
  });
  const running = withTyping(new AbortController().signal, update, () => work.promise);
  await vi.advanceTimersByTimeAsync(9000);
  expect(update).toHaveBeenCalledOnce();
  work.resolve('done');
  expect(await running).toBe('done');
  expect(refreshSignal.aborted).toBe(true);
  pending.reject(new Error('offline'));
  await vi.advanceTimersByTimeAsync(9000);
  expect(update).toHaveBeenCalledOnce();
  expect(vi.getTimerCount()).toBe(0);
});
