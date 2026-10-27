import { afterEach, expect, test, vi } from 'vitest';
import { parseEnv } from 'node:util';
import { RealtimeEvent, type MessagingRequests } from '@chatto/client';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { createTaskContext, runWorkflow, task, Type } from 'runling';
import type { EventSource, WebhookContext } from 'runling/web';
import { IntakeDecision, IntakeInput } from '../tasks/classify-intake.ts';
import { chattoSource, chattoWorkflow } from './source.ts';
import { IncomingMessage } from './message.ts';
import { fakeChatto } from './fake-chatto.ts';

const mocks = vi.hoisted(() => ({
  ready: vi.fn(),
  consumeEvents: vi.fn(),
  addressedMessage: vi.fn(),
  getMessage: vi.fn(),
  close: vi.fn(),
  batchGetUsers: vi.fn(),
  readThread: vi.fn(),
  readMessage: vi.fn(),
  classify: vi.fn(),
  conversation: vi.fn(),
  started: vi.fn(),
  steered: vi.fn()
}));
vi.mock('@chatto/client', async (original) => ({
  ...(await original<typeof import('@chatto/client')>()),
  createClient: () => ({ connect: () => mocks, close: mocks.close }),
  createApi: () => ({
    ready: mocks.ready,
    service: () => ({ batchGetUsers: mocks.batchGetUsers, getMessage: mocks.readMessage }),
    readThread: mocks.readThread
  })
}));
vi.mock('../tasks/classify-intake.ts', async (original) => ({
  ...(await original<typeof import('../tasks/classify-intake.ts')>()),
  createIntakeClassifier: () =>
    task(
      { name: 'Fixture classifier', input: IntakeInput, result: IntakeDecision },
      async (_ctx, input) => mocks.classify(input)
    )
}));
vi.mock('../tasks/conversation.ts', () => ({
  createConversation: () =>
    task(
      {
        name: 'Chatto conversation',
        input: IncomingMessage,
        result: Type.Null(),
        steering: IncomingMessage,
        output: Type.Union([Type.Literal('busy'), Type.Literal('waiting'), Type.Literal('idle')])
      },
      async (_ctx, input) => {
        mocks.conversation(input);
        return null;
      }
    )
}));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetAllMocks();
});

const event = (id: string, roomId = 'intake', actorId = 'human') =>
  new RealtimeEvent({
    id,
    actorId,
    event: { case: 'messagePosted', value: { roomId, bodyPlaintext: 'Images fail' } }
  });

async function run(
  events: RealtimeEvent[],
  {
    rooms = 'intake',
    engage = true,
    allowed = '',
    botAuthor = false,
    missing = false,
    recovered = false,
    addressed,
    storedMessage
  }: {
    storedMessage?: Message;
    rooms?: string;
    engage?: boolean;
    allowed?: string;
    botAuthor?: boolean;
    missing?: boolean;
    recovered?: boolean;
    addressed?: MessagingRequests['addressedMessage'];
  } = {}
) {
  vi.stubEnv('CHATTO_URL', 'https://chatto.example.test');
  vi.stubEnv('CHATTO_API_KEY', 'fixture');
  vi.stubEnv('CHATTO_AGENT_MODEL', 'test/model');
  vi.stubEnv('CHATTO_AUTO_ENGAGE_ROOM_IDS', rooms);
  vi.stubEnv('CHATTO_ALLOWED_USER_ID', allowed);
  vi.stubEnv('CHATTO_MAINTAINER_USER_IDS', 'maintainer');
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '');
  vi.stubEnv('CHATTO_GITHUB_APP_CLIENT_ID', '');
  vi.stubEnv('CHATTO_GITHUB_APP_PRIVATE_KEY_FILE', '');
  vi.stubEnv('CHATTO_GITHUB_REPOSITORY', '');
  mocks.ready.mockResolvedValue({ viewerId: 'bot' });
  mocks.classify.mockResolvedValue({ action: engage ? 'engage' : 'ignore', reason: 'fixture' });
  mocks.addressedMessage.mockImplementation(
    addressed ??
      (async (event: RealtimeEvent) =>
        event.id === 'ping'
          ? {
              id: event.id,
              roomId: 'ordinary',
              authorId: event.actorId,
              body: 'Hello',
              reasons: ['mention']
            }
          : undefined)
  );
  mocks.batchGetUsers.mockResolvedValue({
    users: ['human', 'maintainer'].map((id) => ({
      user: { id, ...(botAuthor ? { bot: {} } : {}) }
    }))
  });
  mocks.readThread.mockImplementation(async ({ threadRootId }: { threadRootId: string }) => ({
    messages: missing
      ? []
      : [
          {
            id: threadRootId,
            authorId: events.find((event) => event.id === threadRootId)?.actorId ?? 'human',
            body: '',
            attachments: [{ contentType: 'image/png' }],
            fromViewer: false
          }
        ],
    olderOmitted: false
  }));
  mocks.readMessage.mockImplementation(
    async ({ roomId, eventId }: { roomId: string; eventId: string }) => ({
      message: missing
        ? undefined
        : (storedMessage ??
          new Message({
            id: eventId,
            roomId,
            actorId: events.find((event) => event.id === eventId)?.actorId ?? 'human',
            attachments: [{ contentType: 'image/png' }]
          }))
    })
  );
  const inputs: unknown[] = [];
  mocks.conversation.mockImplementation((input: unknown) => inputs.push(input));
  const pending: Promise<void>[] = [];
  const start: WebhookContext['start'] = async (root, { input }) => {
    mocks.started(root.name);
    const invocation = createTaskContext().start(root, input);
    invocation.closeSteering();
    const finished = invocation.result.then(
      () => {},
      () => {}
    );
    pending.push(finished);
    return {
      id: 'intake',
      steer: invocation.steer,
      closeSteering: invocation.closeSteering,
      cancel: () => invocation.cancel(),
      finished
    };
  };
  mocks.consumeEvents.mockImplementation(
    async ({ onEvent }: { onEvent: (event: RealtimeEvent) => Promise<void> }) => {
      for (const e of events) await onEvent(e);
      await Promise.all(pending);
    }
  );
  const ctx: Parameters<EventSource>[0] = {
    signal: new AbortController().signal,
    state: new Map(),
    runs: () =>
      recovered
        ? [
            {
              id: 'existing',
              workflow: 'chattoThread',
              input: {
                id: 'root',
                roomId: 'ordinary',
                threadRootId: 'root',
                connection: JSON.stringify(['https://chatto.example.test', 'bot'])
              },
              steer: async (message) => {
                mocks.steered(message);
              },
              closeSteering: () => {},
              cancel: () => {},
              finished: new Promise<void>(() => {})
            }
          ]
        : [],
    dispatch: async (route, input) => {
      await route({ start }, input);
      return [];
    }
  };
  await chattoSource(ctx);
  expect(mocks.close).toHaveBeenCalledOnce();
  return inputs;
}

test('ordinary mentions bypass classification and opted-in roots classify once', async () => {
  const inputs = await run([
    event('root'),
    event('root'),
    event('ping', 'ordinary'),
    event('other', 'ordinary')
  ]);
  expect(inputs).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: 'ping', body: '' }),
      expect.objectContaining({ id: 'root', authorId: 'human', body: '', trigger: 'intake' })
    ])
  );
  expect(inputs).toHaveLength(2);
  // Intake owns the conversation child: the host registers no second run for the report.
  expect(mocks.started.mock.calls).toEqual([['Chatto thread'], ['Chatto thread']]);
  expect(mocks.classify).toHaveBeenCalledOnce();
  expect(mocks.classify).toHaveBeenCalledWith({
    body: '',
    attachmentTypes: ['image/png']
  });
});

test('maintainer roots skip automatic intake while direct mentions still engage', async () => {
  const inputs = await run([
    event('maintainer-report', 'intake', 'maintainer'),
    event('ping', 'intake', 'maintainer')
  ]);
  expect(mocks.classify).not.toHaveBeenCalled();
  expect(inputs).toHaveLength(1);
  expect(inputs[0]).toMatchObject({ id: 'ping' });
});

test.each(['mention', 'reply'])(
  'a maintainer %s always reaches the conversation when participation would ignore it',
  async (kind) => {
    const { createApi } = fakeChatto({
      viewerId: 'bot',
      routes: (router) =>
        router.service(MessageService, {
          getMessage: ({ roomId, eventId }) => ({
            message: {
              id: eventId,
              roomId,
              actorId: eventId === 'bot-message' ? 'bot' : 'maintainer',
              threadRootEventId: 'root',
              body: 'Please continue'
            }
          })
        })
    });
    const api = createApi({ serverUrl: 'https://chatto.example.test', apiKey: 'fixture' });
    const inputs = await run(
      [
        new RealtimeEvent({
          id: 'request',
          actorId: 'maintainer',
          event: {
            case: 'messagePosted',
            value: {
              roomId: 'ordinary',
              threadRootEventId: 'root',
              bodyPlaintext: 'Please continue',
              inReplyTo: kind === 'reply' ? 'bot-message' : '',
              mentions:
                kind === 'mention'
                  ? [{ includesViewer: true, cause: { case: 'direct', value: {} } }]
                  : []
            }
          }
        })
      ],
      {
        rooms: '',
        engage: false,
        addressed: api.addressedMessage.bind(api),
        storedMessage: new Message({
          id: 'request',
          roomId: 'ordinary',
          threadRootEventId: 'root',
          actorId: 'maintainer',
          body: 'Please continue'
        })
      }
    );
    expect(inputs).toEqual([expect.objectContaining({ id: 'request', authorId: 'maintainer' })]);
    expect(mocks.classify).not.toHaveBeenCalled();
  }
);

test('reactions on bot messages reach recovered threads once with the reacting identity', async () => {
  mocks.getMessage.mockResolvedValue({
    id: 'proposal',
    roomId: 'ordinary',
    threadRootId: 'root',
    authorId: 'bot',
    body: 'Shall I file the issue?'
  });
  const reaction = (id: string, actorId = 'human') =>
    new RealtimeEvent({
      id,
      actorId,
      event: {
        case: 'reactionAdded',
        value: { roomId: 'ordinary', messageEventId: 'proposal', emoji: '+1' }
      }
    });
  await run(
    [
      reaction('approval'),
      reaction('approval'),
      reaction('own', 'bot'),
      reaction('blocked', 'other')
    ],
    { recovered: true, allowed: 'human' }
  );
  expect(mocks.steered).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({
      id: 'approval',
      authorId: 'human',
      threadRootId: 'root',
      reaction: { emoji: '+1', messageId: 'proposal', body: 'Shall I file the issue?' }
    })
  );
  expect(mocks.classify).not.toHaveBeenCalled();
  expect(mocks.started).not.toHaveBeenCalled();
});

test.each([true, false])(
  'a reaction reopens a conversation only when classified as engage=%s',
  async (engage) => {
    mocks.getMessage.mockResolvedValue({
      id: 'proposal',
      roomId: 'ordinary',
      threadRootId: 'root',
      authorId: 'bot',
      body: 'Shall I file the issue?'
    });
    const inputs = await run(
      [
        new RealtimeEvent({
          id: 'approval',
          actorId: 'human',
          event: {
            case: 'reactionAdded',
            value: { roomId: 'ordinary', messageEventId: 'proposal', emoji: '+1' }
          }
        })
      ],
      { engage }
    );
    expect(inputs).toEqual(
      engage
        ? [
            expect.objectContaining({
              id: 'approval',
              authorId: 'human',
              reaction: expect.objectContaining({ messageId: 'proposal' })
            })
          ]
        : []
    );
    expect(mocks.classify).toHaveBeenCalledExactlyOnceWith({
      context: [{ role: 'human', body: '' }],
      reaction: { emoji: '+1', messageId: 'proposal', body: 'Shall I file the issue?' }
    });
    expect(mocks.readThread).toHaveBeenCalledWith(
      { roomId: 'ordinary', threadRootId: 'root' },
      { signal: expect.any(AbortSignal), limit: 20 }
    );
  }
);

test.each([RoomKind.DM, RoomKind.CHANNEL])(
  'thread addressing preserves DM requests and channel follow-ups for room kind %s',
  async (roomKind) => {
    const trigger = roomKind === RoomKind.DM ? undefined : 'follow-up';
    const { createApi } = fakeChatto({
      viewerId: 'bot',
      routes: (router) =>
        router.service(MessageService, {
          getMessage: ({ roomId, eventId }) => ({
            message: {
              id: eventId,
              roomId,
              threadRootEventId: 'root',
              actorId: eventId === 'bot-message' ? 'bot' : 'human',
              body: 'Previous message'
            }
          })
        })
    });
    const api = createApi({ serverUrl: 'https://chatto.example.test', apiKey: 'fixture' });
    const post = (id: string, inReplyTo = '', threadRootEventId = 'root') =>
      new RealtimeEvent({
        id,
        actorId: 'human',
        event: {
          case: 'messagePosted',
          value: {
            roomId: 'ordinary',
            roomKind,
            threadRootEventId,
            inReplyTo,
            bodyPlaintext: 'Hello',
            mentions:
              id === 'mention'
                ? [{ includesViewer: true, cause: { case: 'direct', value: {} } }]
                : []
          }
        }
      });
    await run(
      [
        post('ordinary'),
        post('ordinary'),
        post('human-reply', 'human-message'),
        post('mention'),
        post('bot-reply', 'bot-message'),
        ...(roomKind === RoomKind.CHANNEL ? [post('unwatched', '', 'other')] : [])
      ],
      { rooms: '', recovered: true, addressed: api.addressedMessage.bind(api) }
    );
    expect(
      mocks.steered.mock.calls.map(([message]) => ({ id: message.id, trigger: message.trigger }))
    ).toEqual([
      { id: 'ordinary', trigger },
      { id: 'human-reply', trigger },
      { id: 'mention', trigger: undefined },
      { id: 'bot-reply', trigger: undefined }
    ]);
    expect(mocks.started).not.toHaveBeenCalled();
  }
);

test('a new DM root still starts a direct conversation', async () => {
  const { createApi } = fakeChatto({ viewerId: 'bot', routes: () => {} });
  const api = createApi({ serverUrl: 'https://chatto.example.test', apiKey: 'fixture' });
  const inputs = await run(
    [
      new RealtimeEvent({
        id: 'dm-root',
        actorId: 'human',
        event: {
          case: 'messagePosted',
          value: {
            roomId: 'ordinary',
            roomKind: RoomKind.DM,
            bodyPlaintext: 'Help me set up Chatto'
          }
        }
      })
    ],
    { rooms: '', addressed: api.addressedMessage.bind(api) }
  );
  expect(inputs).toEqual([
    expect.objectContaining({
      id: 'dm-root',
      threadRootId: 'dm-root',
      body: ''
    })
  ]);
  expect((inputs[0] as IncomingMessage).trigger).toBeUndefined();
  expect(mocks.classify).not.toHaveBeenCalled();
});

test('realtime replies in ordinary rooms wake recovered active threads without starting runs', async () => {
  const reply = (id: string, threadRootEventId = 'root', actorId = 'human') =>
    new RealtimeEvent({
      id,
      actorId,
      event: {
        case: 'messagePosted',
        value: { roomId: 'ordinary', threadRootEventId, bodyPlaintext: '0.5' }
      }
    });
  await run(
    [reply('one'), reply('one'), reply('unwatched', 'other'), reply('own', 'root', 'bot')],
    { rooms: '', recovered: true }
  );
  expect(mocks.steered).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ id: 'one', threadRootId: 'root', trigger: 'follow-up' })
  );
  expect(mocks.started).not.toHaveBeenCalled();
  expect(mocks.classify).not.toHaveBeenCalled();
});

test('the allowed-user restriction also excludes realtime follow-up notifications', async () => {
  const reply = new RealtimeEvent({
    id: 'reply',
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: { roomId: 'ordinary', threadRootEventId: 'root', bodyPlaintext: '0.5' }
    }
  });
  await run([reply], { recovered: true, allowed: 'someone-else' });
  expect(mocks.steered).not.toHaveBeenCalled();
});

test('a recovered thread cannot use a different configured server or bot identity', async () => {
  await run([]);
  const result = await runWorkflow(chattoWorkflow, {
    input: {
      id: 'old',
      roomId: 'room',
      threadRootId: 'old',
      authorId: 'human',
      body: 'old request',
      connection: 'another bot'
    },
    run: { id: 'recovered', resumed: true }
  });
  expect(result.ok).toBe(false);
  expect(result.error).toContain('different Chatto connection');
  expect(mocks.conversation).not.toHaveBeenCalled();
  expect(mocks.classify).not.toHaveBeenCalled();
});

test.each([
  'CHATTO_AUTO_ENGAGE_ROOM_IDS="R-one,R-two"',
  'CHATTO_AUTO_ENGAGE_ROOM_IDS=R-one,R-two',
  'CHATTO_AUTO_ENGAGE_ROOM_IDS="R-one, R-two"',
  'CHATTO_AUTO_ENGAGE_ROOM_IDS="R-one R-two"'
])('routes new reports from both configured rooms: %s', async (line) => {
  // Parse the .env syntax before exercising the real settings, source, router, and workflow.
  // Only Chatto transport and model calls are replaced by fixtures.
  const rooms = parseEnv(line).CHATTO_AUTO_ENGAGE_ROOM_IDS!;
  const inputs = await run(
    [event('first', 'R-one'), event('second', 'R-two'), event('excluded', 'R-three')],
    { rooms }
  );
  expect(inputs).toHaveLength(2);
  expect(inputs).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: 'first', roomId: 'R-one', trigger: 'intake' }),
      expect.objectContaining({ id: 'second', roomId: 'R-two', trigger: 'intake' })
    ])
  );
  expect(mocks.classify).toHaveBeenCalledTimes(2);
  expect(mocks.started.mock.calls).toEqual([['Chatto thread'], ['Chatto thread']]);
});

test.each([{ rooms: '' }, { allowed: 'someone-else' }, { botAuthor: true }, { missing: true }])(
  'does not classify excluded posts: %j',
  async (options) => {
    expect(await run([event('root')], options)).toEqual([]);
    expect(mocks.classify).not.toHaveBeenCalled();
  }
);

test('an addressed reply outside the recent thread window is read by exact ID', async () => {
  const inputs = await run([event('old-reply', 'ordinary')], {
    addressed: async () => ({
      id: 'old-reply',
      roomId: 'ordinary',
      threadRootId: 'root',
      authorId: 'human',
      body: 'Stale event body',
      reasons: ['mention']
    }),
    storedMessage: new Message({
      id: 'old-reply',
      roomId: 'ordinary',
      threadRootEventId: 'root',
      actorId: 'human',
      body: 'Current question',
      attachments: [{ contentType: 'image/png' }]
    })
  });
  // The recent thread fixture contains only the root, never this older reply.
  expect(inputs).toEqual([
    expect.objectContaining({ id: 'old-reply', body: 'Current question', threadRootId: 'root' })
  ]);
  expect(mocks.classify).not.toHaveBeenCalled();
  expect(mocks.readMessage).toHaveBeenCalledWith(
    { roomId: 'ordinary', eventId: 'old-reply' },
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  );
});

test.each([
  { id: 'wrong-id' },
  { roomId: 'wrong-room' },
  { threadRootEventId: 'wrong-thread' },
  { actorId: 'wrong-author' },
  { deletedAt: { seconds: 1n, nanos: 0 } },
  { body: undefined, attachments: [] }
])('exact lookup rejects an invalid or unavailable message: %o', async (invalid) => {
  const inputs = await run([event('ping', 'ordinary')], {
    storedMessage: new Message({
      id: 'ping',
      roomId: 'ordinary',
      actorId: 'human',
      attachments: [{ contentType: 'image/png' }],
      ...invalid
    })
  });
  expect(inputs).toEqual([]);
  expect(mocks.classify).not.toHaveBeenCalled();
});
