/** Verify private inputs and schema validation through Runling's classifier. */
import { afterEach, expect, test, vi } from 'vitest';
import { createTaskContext, log, type RunlingAgent, type AgentOptions } from 'runling';
import { createParticipationClassifier } from './participation.ts';
import { parseSettings } from './settings.ts';

const mock = vi.hoisted(() => ({ run: vi.fn(), create: vi.fn() }));
vi.mock('runling', async (original) => {
  const runtime = await original<typeof import('runling')>();
  return {
    ...runtime,
    createClassifier: (options: Parameters<typeof runtime.createClassifier>[0]) =>
      runtime.createClassifier(options, mock.create)
  };
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.resetAllMocks();
});

function setup(summary = '{"action":"ignore","reason":"acknowledgement"}') {
  mock.run.mockResolvedValue({ summary });
  mock.create.mockImplementation(
    async (_options: AgentOptions) =>
      ({
        run: mock.run,
        [Symbol.asyncDispose]: async () => {}
      }) as unknown as RunlingAgent
  );
  const settings = parseSettings(
    '[chatto]\nurl="https://chat.example.com"\napi_key="unused"\n[agent]\nmodel="test/model"',
    'botto.toml'
  );
  return createParticipationClassifier(settings);
}

test('reaction inputs retain their target and cannot produce a reaction to the bot itself', async () => {
  const classify = setup('{"action":"react","emoji":"tada","reason":"celebration"}');
  const reaction = {
    id: 'reaction-event',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: '',
    reasons: [],
    reaction: { messageId: 'answer', emoji: 'confused' }
  };
  await expect(
    classify(
      createTaskContext(),
      {
        messages: [{ id: 'answer', author: 'bot-id', fromViewer: true, text: 'The answer is 42.' }],
        olderContextOmitted: false
      },
      [reaction],
      new AbortController().signal
    )
  ).rejects.toThrow();
  expect(JSON.parse(mock.run.mock.calls[0]![1])).toEqual({
    context: [{ id: 'answer', author: 'bot', text: 'The answer is 42.' }],
    currentMessages: [{ author: 'human', reaction: reaction.reaction }]
  });
});

test('sends fresh current messages once, identifies the bot, and omits image bytes', async () => {
  vi.spyOn(console, 'info').mockImplementation(() => {});
  const classify = setup();
  const ctx = createTaskContext();
  expect(
    await classify(
      ctx,
      {
        messages: [
          { id: 'bot-message', author: 'bot-id', fromViewer: true, text: 'What can I help with?' },
          {
            id: 'current',
            author: 'human',
            text: 'Edited question',
            attachments: [
              { id: 'attachment', contentType: 'image/png', filename: 'private-name.png' }
            ]
          }
        ],
        olderContextOmitted: false,
        images: [
          {
            messageId: 'current',
            attachmentId: 'attachment',
            mimeType: 'image/png',
            data: 'private-image'
          }
        ]
      },
      [
        {
          id: 'current',
          roomId: 'room',
          threadRootId: 'thread',
          authorId: 'human',
          body: 'Stale question',
          reasons: []
        }
      ],
      new AbortController().signal
    )
  ).toEqual({ action: 'ignore' });
  expect(mock.create).toHaveBeenCalledWith(
    expect.objectContaining({
      model: 'openrouter/z-ai/glm-5.3-flash',
      thinkingLevel: 'low',
      tools: [],
      logContent: false
    })
  );
  const payload = JSON.parse(mock.run.mock.calls[0]![1]);
  expect(payload).toEqual({
    context: [{ id: 'bot-message', author: 'bot', text: 'What can I help with?' }],
    currentMessages: [
      { author: 'human', text: 'Edited question', attachments: [{ contentType: 'image/png' }] }
    ]
  });
  expect(mock.run).toHaveBeenCalledOnce();
});

test.each(['tada', 'thinking_face', '+1', 'rocket'])(
  'accepts a model-selected emoji: %s',
  async (emoji) => {
    const classify = setup(JSON.stringify({ action: 'react', emoji, reason: 'celebration' }));
    expect(
      await classify(
        createTaskContext(),
        { messages: [], olderContextOmitted: false },
        [],
        new AbortController().signal
      )
    ).toEqual({ action: 'react', emoji });
  }
);

test.each([
  'not JSON',
  '{"action":"react","reason":"celebration"}',
  '{"action":"react","emoji":"","reason":"celebration"}',
  '{"action":"react","emoji":":tada:","reason":"celebration"}',
  '{"action":"react","emoji":"tada","messageId":"other","reason":"celebration"}',
  '{"action":"reply","emoji":"tada","reason":"follow_up"}',
  '{"action":"post","reason":"follow_up"}'
])('rejects invalid decisions before they can cause effects: %s', async (summary) => {
  const classify = setup(summary);
  await expect(
    classify(
      createTaskContext(),
      { messages: [], olderContextOmitted: false },
      [],
      new AbortController().signal
    )
  ).rejects.toThrow();
});

test('logs only the validated decision, safe reason label, emoji, and duration', async () => {
  const info = vi.spyOn(log, 'info');
  const classify = setup('{"action":"react","emoji":"tada","reason":"celebration"}');
  await classify(
    createTaskContext(),
    {
      messages: [{ id: 'private-id', author: 'private-person', text: 'private-message' }],
      olderContextOmitted: false
    },
    [],
    new AbortController().signal
  );
  expect(info).toHaveBeenCalledWith('Participation: react — Good news needs no text', {
    emoji: 'tada',
    durationMs: expect.any(Number)
  });
  expect(JSON.stringify(info.mock.calls)).not.toContain('private-');
});

test.each(['{"action":"ignore"}', '{"action":"ignore","reason":"private-person said something"}'])(
  'rejects missing or free-text reasons without logging them: %s',
  async (summary) => {
    const info = vi.spyOn(log, 'info');
    await expect(
      setup(summary)(
        createTaskContext(),
        { messages: [], olderContextOmitted: false },
        [],
        new AbortController().signal
      )
    ).rejects.toThrow();
    expect(JSON.stringify(info.mock.calls)).not.toContain('Participation:');
    expect(JSON.stringify(info.mock.calls)).not.toContain('private-person');
  }
);
