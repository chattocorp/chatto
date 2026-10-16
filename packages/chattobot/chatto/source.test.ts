import { afterEach, expect, test, vi } from 'vitest';
import { RealtimeEvent } from '@chatto/client';
import { createTaskContext, task, Type } from 'runling';
import type { EventSource, WebhookContext } from 'runling/web';
import { IntakeDecision, IntakeInput } from '../tasks/classify-intake.ts';
import { chattoSource } from './source.ts';
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
  started: vi.fn()
}));
vi.mock('@chatto/client', async (original) => ({
  ...(await original<typeof import('@chatto/client')>()),
  createClient: () => ({ connect: () => mocks, close: mocks.close }),
  createApi: () => ({
    service: () => ({ batchGetUsers: mocks.batchGetUsers }),
    readThread: mocks.readThread
  })
}));
vi.mock('../tasks/classify-intake.ts', async (original) => ({
  ...(await original<typeof import('../tasks/classify-intake.ts')>()),
  createIntakeClassifier: () =>
    task(
      { name: 'Fixture classifier', input: IntakeInput, output: IntakeDecision },
      async (_ctx, input) => mocks.classify(input)
    )
}));
vi.mock('../tasks/conversation.ts', () => ({
  createConversation: () =>
    task(
      {
        name: 'Chatto conversation',
        input: IncomingMessage,
        output: Type.Null(),
        receives: IncomingMessage,
        emits: Type.Union([Type.Literal('busy'), Type.Literal('waiting'), Type.Literal('idle')])
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
  { rooms = 'intake', allowed = '', botAuthor = false, missing = false } = {}
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
    invocation.closeInput();
    const finished = invocation.result.then(
      () => {},
      () => {}
    );
    pending.push(finished);
    return {
      id: 'intake',
      send: invocation.send,
      closeInput: invocation.closeInput,
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

test.each([{ rooms: '' }, { allowed: 'someone-else' }, { botAuthor: true }, { missing: true }])(
  'does not classify excluded posts: %j',
  async (options) => {
    expect(await run([event('root')], options)).toEqual([]);
    expect(mocks.classify).not.toHaveBeenCalled();
  }
);
