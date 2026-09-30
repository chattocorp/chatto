import { afterEach, expect, test, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { RealtimeEvent, RoomKind } from '@chatto/client';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { EventSourceContext } from 'runling/web';
import { chattoSource } from './realtime.ts';
import { RegistrationError } from './routing.ts';
import type { createChattoBot } from '../workflows/chat.ts';
import { fakeChatto, settle, type FakeChattoSetup } from './fake-chatto.ts';

const mocks = vi.hoisted(() => ({
  setup: undefined as unknown as FakeChattoSetup,
  getMessage: vi.fn(),
  bot: vi.fn((_options: Parameters<typeof createChattoBot>[0]) => ({ route: () => {} }))
}));
let chatto = fakeChatto({ viewerId: 'bot', routes: () => {} });
vi.mock('@chatto/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client')>()),
  createClient: () => chatto.createClient(),
  createApi: (options: Parameters<typeof chatto.createApi>[0]) => chatto.createApi(options)
}));
vi.mock('../workflows/chat.ts', () => ({ createChattoBot: mocks.bot }));

function useServer(viewerId: FakeChattoSetup['viewerId'] = 'bot') {
  chatto = fakeChatto({
    viewerId,
    routes(router) {
      router.service(MessageService, {
        getMessage: mocks.getMessage,
        createMessage: () => ({ message: { id: 'reply' } }),
        addReaction: () => ({})
      });
      router.service(RoomService, { refreshTypingIndicator: () => ({}) });
      router.service(ThreadService, { getThreadEvents: () => ({ page: { events: [] } }) });
    }
  });
}
useServer();

afterEach(() => {
  vi.unstubAllEnvs();
  vi.clearAllMocks();
  useServer();
});

function context(dispatch = vi.fn().mockResolvedValue([])) {
  const controller = new AbortController();
  const ctx: EventSourceContext = { signal: controller.signal, state: new Map(), dispatch };
  return { ctx, controller };
}

/**
 * Run one source generation: wait until it listens, deliver `events`, let
 * handlers finish, then end the generation.
 */
async function generation(
  ctx: EventSourceContext,
  controller: AbortController,
  events: RealtimeEvent[] = []
) {
  const running = chattoSource(ctx);
  await vi.waitFor(() => expect(chatto.connections.at(-1)?.listening).toBe(true));
  for (const event of events) chatto.connections.at(-1)!.emit(event);
  await settle();
  controller.abort();
  await running;
}

const messageEvent = (
  id: string,
  actorId: string,
  value: Partial<{
    roomKind: RoomKind;
    bodyPlaintext: string;
    threadRootEventId: string;
    inReplyTo: string;
    mentions: { includesViewer: boolean }[];
  }>
) =>
  new RealtimeEvent({
    id,
    actorId,
    event: {
      case: 'messagePosted',
      value: { roomId: 'room', roomKind: RoomKind.CHANNEL, bodyPlaintext: 'hello', ...value }
    }
  });

test.each(['bot', 'human', 'wrong-thread', 'missing', 'unavailable'])(
  'verifies unmentioned replies against %s targets',
  async (kind) => {
    vi.stubEnv('CHATTO_URL', 'https://chat.example');
    vi.stubEnv('CHATTO_API_KEY', 'key');
    vi.stubEnv('CHATTO_ALLOWED_USER_ID', 'allowed');
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
    mocks.getMessage.mockImplementation(() => {
      if (kind === 'unavailable') throw new ConnectError('private error', Code.Unavailable);
      return {
        message:
          kind === 'missing'
            ? undefined
            : {
                id: 'reply-target',
                actorId: kind === 'human' ? 'human' : 'bot',
                roomId: 'room',
                threadRootEventId: kind === 'wrong-thread' ? 'other' : 'root'
              }
      };
    });
    const { ctx, controller } = context();
    try {
      await generation(
        ctx,
        controller,
        ['other', 'bot', 'allowed'].map((actorId) =>
          messageEvent(actorId, actorId, {
            threadRootEventId: 'root',
            inReplyTo: 'reply-target',
            bodyPlaintext: 'Follow-up'
          })
        )
      );
      expect(mocks.getMessage).toHaveBeenCalledOnce(); // Filter other senders before lookup.
      expect(ctx.dispatch).toHaveBeenCalledTimes(kind === 'bot' ? 1 : 0);
      if (kind === 'bot')
        expect(vi.mocked(ctx.dispatch).mock.calls[0]![1]).toMatchObject({ triggers: ['reply'] });
      expect(JSON.stringify(warning.mock.calls)).not.toContain('private error');
    } finally {
      warning.mockRestore();
    }
  }
);

test.each([undefined, '', '  allowed-user  '])(
  'filters senders before routing with allowed user %s',
  async (configured) => {
    vi.stubEnv('CHATTO_URL', 'https://chat.example');
    vi.stubEnv('CHATTO_API_KEY', 'key');
    vi.stubEnv('CHATTO_ALLOWED_USER_ID', configured);
    const events: RealtimeEvent[] = [];
    for (const actorId of ['allowed-user', 'other-user'])
      for (const roomKind of [RoomKind.DM, RoomKind.CHANNEL])
        for (const bodyPlaintext of ['hello', 'follow-up', '/cancel'])
          events.push(
            messageEvent(`${actorId}-${roomKind}-${bodyPlaintext}`, actorId, {
              roomKind,
              bodyPlaintext,
              threadRootEventId: bodyPlaintext === 'hello' ? '' : 'existing-thread',
              mentions: [{ includesViewer: true }]
            })
          );
    const { ctx, controller } = context();
    await generation(ctx, controller, events);
    expect(ctx.dispatch).toHaveBeenCalledTimes(configured ? 6 : 12);
    if (configured) {
      for (const [, delivery] of vi.mocked(ctx.dispatch).mock.calls)
        expect((delivery as { message: { author_id: string } }).message.author_id).toBe(
          'allowed-user'
        );
    }
  }
);

test('retains conversations across reloads, isolates a new identity, and closes each connection', async () => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'first-key');
  const state = new Map<string, unknown>();
  const run = async () => {
    const controller = new AbortController();
    await generation({ signal: controller.signal, state, dispatch: vi.fn() }, controller);
  };
  const warning = vi.spyOn(console, 'warn').mockImplementation(() => {});
  await run();
  expect(warning).not.toHaveBeenCalled();
  await run();
  expect(warning).toHaveBeenCalledWith(
    'ChattoBot reloaded: messages sent during the reload are not replayed.'
  );
  warning.mockRestore();
  const firstState = mocks.bot.mock.calls[0]![0]!.state;
  expect(mocks.bot.mock.calls[1]![0]!.state).toBe(firstState);

  vi.stubEnv('CHATTO_API_KEY', 'second-key');
  await run();
  expect(mocks.bot.mock.calls[2]![0]!.state).toBe(firstState);
  expect(chatto.connections.map((connection) => connection.options.apiKey)).toEqual([
    'first-key',
    'first-key',
    'second-key'
  ]);
  expect(chatto.connections.every((connection) => connection.closed)).toBe(true);

  useServer('different-bot');
  await run();
  expect(mocks.bot.mock.calls[3]![0]!.state).not.toBe(firstState);
});

test('closes the connection when the server rejects the key', async () => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'key');
  useServer(async () => {
    throw new Error('Chatto rejected the API key');
  });
  const { ctx } = context();
  await expect(chattoSource(ctx)).rejects.toThrow('rejected the API key');
  expect(chatto.connections[0]!.closed).toBe(true);
  expect(mocks.bot).not.toHaveBeenCalled();
});

test('passes implementation configuration through the realtime source and captures it per generation', async () => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'key');
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '/configured/chatto');
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', 'example/chatto');
  vi.stubEnv('CHATTO_SOURCE_REF', 'main');
  vi.stubEnv('CHATTO_IMPLEMENTATION_MODEL', 'test/worker');
  vi.stubEnv('CHATTO_MAINTAINER_USER_IDS', ' alice, bob ');
  const run = async () => {
    const { ctx, controller } = context();
    await generation(ctx, controller);
  };
  await run();
  const original = mocks.bot.mock.calls[0]![0]!;
  expect(original.implementation).toEqual({
    directory: '/configured/chatto',
    repository: 'example/chatto',
    baseBranch: 'main',
    model: 'test/worker',
    thinkingLevel: 'medium'
  });
  expect(original.investigation?.baseRef).toBe('refs/remotes/origin/main');
  expect(original.maintainers).toEqual(['alice', 'bob']);
  vi.stubEnv('CHATTO_SOURCE_REF', 'next');
  await run();
  expect(mocks.bot.mock.calls[1]![0]!.implementation?.baseBranch).toBe('next');
  expect(original.implementation?.baseBranch).toBe('main');
  vi.stubEnv('CHATTO_IMPLEMENTATION_REPOSITORY', '');
  await run();
  expect(mocks.bot.mock.calls[2]![0]!.implementation).toBeUndefined();
});

test('existing conversation callbacks keep their server and credentials after reload', async () => {
  vi.stubEnv('CHATTO_URL', 'https://original.example');
  vi.stubEnv('CHATTO_API_KEY', 'original-key');
  const state = new Map<string, unknown>();
  const run = async () => {
    const controller = new AbortController();
    await generation({ signal: controller.signal, state, dispatch: vi.fn() }, controller);
  };
  await run();
  const original = mocks.bot.mock.calls[0]![0]!;
  vi.stubEnv('CHATTO_URL', 'https://replacement.example');
  vi.stubEnv('CHATTO_API_KEY', 'replacement-key');
  await run();
  const destination = { roomId: 'room', threadRootId: 'root' };
  const delivery = {
    version: 1 as const,
    id: 'message',
    type: 'message.created' as const,
    triggers: ['direct_message'],
    occurred_at: '',
    bot_id: 'bot',
    room_id: 'room',
    thread_root_id: 'root',
    message: { id: 'message', author_id: 'human', body: 'hello' }
  };
  const signal = new AbortController().signal;
  await original.post!(destination, 'reply', signal);
  await original.typing!(destination, signal);
  await original.readThread!(delivery, signal);
  await original.acknowledge!(delivery, signal);
  // The generation's connection is closed; its runs still reach the server.
  expect(chatto.connections.every((connection) => connection.closed)).toBe(true);
  const [first, replacement] = chatto.apis;
  expect(first!.options).toEqual({
    serverUrl: 'https://original.example',
    apiKey: 'original-key',
    viewerId: 'bot'
  });
  expect(first!.calls).toEqual([
    'MessageService/CreateMessage',
    'RoomService/RefreshTypingIndicator',
    'ThreadService/GetThreadEvents',
    'MessageService/AddReaction'
  ]);
  expect(replacement!.calls).toEqual([]);
});

test.each(['recover', 'exhaust', 'abort', 'other'])('registration retry: %s', async (mode) => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'key');
  const controller = new AbortController();
  const failure = new Error('Event routing failed', {
    cause: new RegistrationError(new Error('disk'))
  });
  const dispatch = vi.fn().mockImplementation(async () => {
    if (mode === 'abort') controller.abort();
    if (mode === 'other') throw new Error('Other routing failure');
    if (mode === 'recover' && dispatch.mock.calls.length === 2) return [];
    throw failure;
  });
  const result = chattoSource({ signal: controller.signal, state: new Map(), dispatch });
  await vi.waitFor(() => expect(chatto.connections.at(-1)?.listening).toBe(true));
  chatto.connections
    .at(-1)!
    .emit(messageEvent('message', 'human', { roomKind: RoomKind.DM, bodyPlaintext: 'hello' }));
  if (mode === 'recover') {
    await vi.waitFor(() => expect(dispatch).toHaveBeenCalledTimes(2));
    controller.abort();
    await expect(result).resolves.toBeUndefined();
  } else await expect(result).rejects.toBeInstanceOf(Error);
  expect(dispatch).toHaveBeenCalledTimes(mode === 'recover' ? 2 : mode === 'exhaust' ? 3 : 1);
  expect(chatto.connections.at(-1)!.closed).toBe(true);
});

test.each([
  [
    { CHATTO_IMPLEMENTATION_REPOSITORY: 'example/chatto', CHATTO_SOURCE_DIRECTORY: '' },
    'CHATTO_IMPLEMENTATION_REPOSITORY requires CHATTO_SOURCE_DIRECTORY'
  ],
  [{ CHATTO_URL: 'not a url' }, 'CHATTO_URL must be an HTTP or HTTPS URL without credentials'],
  [
    { CHATTO_SOURCE_DIRECTORY: '/configured/chatto', CHATTO_MAINTAINER_USER_IDS: '' },
    'Source investigation, implementation, and GitHub access require CHATTO_MAINTAINER_USER_IDS'
  ],
  [
    { CHATTO_MAINTAINER_USER_IDS: 'alice; drop table' },
    'CHATTO_MAINTAINER_USER_IDS must list Chatto user IDs separated by commas'
  ],
  [
    { CHATTO_CLOUDFLARE_ACCOUNT_ID: '0123456789abcdef0123456789abcdef' },
    'Set both CHATTO_CLOUDFLARE_ACCOUNT_ID and CHATTO_CLOUDFLARE_API_TOKEN, or neither'
  ],
  [
    { CHATTO_GITHUB_APP_CLIENT_ID: 'Iv23liExample1' },
    'Set both CHATTO_GITHUB_APP_CLIENT_ID and CHATTO_GITHUB_APP_PRIVATE_KEY_FILE, or neither'
  ],
  [
    {
      CHATTO_GITHUB_APP_CLIENT_ID: 'Iv23liExample1',
      CHATTO_GITHUB_APP_PRIVATE_KEY_FILE: '/nonexistent/secret-key.pem'
    },
    'CHATTO_GITHUB_APP_PRIVATE_KEY_FILE must name a readable GitHub App private key (.pem)'
  ],
  [
    { CHATTO_URL: 'https://user:secret@chat.example' },
    'CHATTO_URL must be an HTTP or HTTPS URL without credentials'
  ]
])('reports invalid settings before contacting the server: %j', async (env, message) => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'key');
  for (const [name, value] of Object.entries(env)) vi.stubEnv(name, value);
  const error = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { ctx } = context();
  await expect(chattoSource(ctx)).rejects.toThrow(message);
  expect(error).toHaveBeenCalledWith(`ChattoBot configuration error: ${message}`);
  expect(JSON.stringify(error.mock.calls)).not.toContain('secret');
  expect(chatto.createClient).not.toHaveBeenCalled();
  expect(mocks.bot).not.toHaveBeenCalled();
});
