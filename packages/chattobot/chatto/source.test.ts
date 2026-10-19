import { afterEach, expect, test, vi } from 'vitest';
import { parseEnv } from 'node:util';
import { RealtimeEvent } from '@chatto/client';
import { createTaskContext, runWorkflow, task, Type } from 'runling';
import type { EventSource, WebhookContext } from 'runling/web';
import { IntakeDecision, IntakeInput } from '../tasks/classify-intake.ts';
import { chattoSource, chattoWorkflow } from './source.ts';
import { IncomingMessage } from './message.ts';

const mocks = vi.hoisted(() => ({
  ready: vi.fn(),
  consumeEvents: vi.fn(),
  addressedMessage: vi.fn(),
  close: vi.fn(),
  batchGetUsers: vi.fn(),
  readThread: vi.fn(),
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
    service: () => ({ batchGetUsers: mocks.batchGetUsers }),
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
  { rooms = 'intake', allowed = '', botAuthor = false, missing = false, recovered = false } = {}
) {
  vi.stubEnv('CHATTO_URL', 'https://chatto.example.test');
  vi.stubEnv('CHATTO_API_KEY', 'fixture');
  vi.stubEnv('CHATTO_AGENT_MODEL', 'test/model');
  vi.stubEnv('CHATTO_AUTO_ENGAGE_ROOM_IDS', rooms);
  vi.stubEnv('CHATTO_ALLOWED_USER_ID', allowed);
  vi.stubEnv('CHATTO_SOURCE_DIRECTORY', '');
  vi.stubEnv('CHATTO_GITHUB_APP_CLIENT_ID', '');
  vi.stubEnv('CHATTO_GITHUB_APP_PRIVATE_KEY_FILE', '');
  vi.stubEnv('CHATTO_GITHUB_REPOSITORY', '');
  mocks.ready.mockResolvedValue({ viewerId: 'bot' });
  mocks.classify.mockResolvedValue({ engage: true, reason: 'bug' });
  mocks.addressedMessage.mockImplementation(async (event: RealtimeEvent) =>
    event.id === 'ping'
      ? {
          id: event.id,
          roomId: 'ordinary',
          authorId: event.actorId,
          body: 'Hello',
          reasons: ['mention']
        }
      : undefined
  );
  mocks.batchGetUsers.mockResolvedValue({
    users: [{ user: { id: 'human', ...(botAuthor ? { bot: {} } : {}) } }]
  });
  mocks.readThread.mockImplementation(async ({ threadRootId }: { threadRootId: string }) => ({
    messages: missing
      ? []
      : [
          {
            id: threadRootId,
            authorId: 'human',
            body: '',
            attachments: [{ contentType: 'image/png' }],
            fromViewer: false
          }
        ],
    olderOmitted: false
  }));
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

test('ordinary mentions bypass intake and opted-in roots classify once before conversation', async () => {
  const inputs = await run([
    event('root'),
    event('root'),
    event('ping', 'ordinary'),
    event('other', 'ordinary')
  ]);
  expect(inputs).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: 'ping', body: 'Hello' }),
      expect.objectContaining({ id: 'root', authorId: 'human', body: '', trigger: 'intake' })
    ])
  );
  expect(inputs).toHaveLength(2);
  // Intake owns the conversation child: the host registers no second run for the report.
  expect(mocks.started.mock.calls).toEqual([['Chatto thread'], ['Chatto thread']]);
  expect(mocks.classify).toHaveBeenCalledExactlyOnceWith({
    body: '',
    attachmentTypes: ['image/png']
  });
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
