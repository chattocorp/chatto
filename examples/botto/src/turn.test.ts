/** Exercise inbox, shared client timeline, Pi input, and reply routing together. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { Destination, Server } from '@chatto/client';
import type { IncomingMessage } from './messages.ts';
import { Inbox } from './inbox.ts';
import { createTurnHandler } from './turn.ts';
import { parseSettings } from './settings.ts';
import { contextFixture, page, row } from './context.fixture.ts';
import { hasTranscript } from './messages.ts';
import { createTaskContext } from 'runling';
import { createTitleWriter } from './title.ts';
import type { createParticipationClassifier } from './participation.ts';
import { withTyping } from '@chatto/client/messaging/typing';

const participation = vi.hoisted(() => vi.fn<ReturnType<typeof createParticipationClassifier>>());
vi.mock('./participation.ts', async (original) => ({
  ...(await original<typeof import('./participation.ts')>()),
  createParticipationClassifier: () => participation
}));

const naming = vi.hoisted(() => vi.fn<ReturnType<typeof createTitleWriter>>());
vi.mock('./title.ts', () => ({ createTitleWriter: () => naming }));

vi.mock('./messages.ts', () => ({ hasTranscript: vi.fn() }));

beforeEach(() => {
  participation.mockReset().mockResolvedValue({ action: 'reply' });
  naming.mockReset().mockResolvedValue(undefined);
  vi.mocked(hasTranscript).mockReset().mockResolvedValue(false);
  vi.useFakeTimers();
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
});
const fixtures: { dispose(): void }[] = [];
afterEach(() => {
  for (const rooms of fixtures.splice(0)) rooms.dispose();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function setup(beforeEffects: (messages: readonly IncomingMessage[]) => void = () => {}) {
  const settings = parseSettings(
    '[chatto]\nurl="https://chat.example.com"\napi_key="test"\n[agent]\nmodel="test/model"',
    'botto.toml',
    {}
  );
  const fixture = contextFixture();
  fixtures.push(fixture.rooms);
  const server = {
    ...fixture.server,
    getMessage: vi.fn<Server['getMessage']>().mockResolvedValue(undefined),
    readAttachment: vi.fn<Server['readAttachment']>(),
    addReaction: vi.fn<Server['addReaction']>().mockResolvedValue(undefined),
    postMessage: vi.fn<Server['postMessage']>().mockResolvedValue({ ids: ['reply'] }),
    withTyping: async <T>(_destination: Destination, work: () => Promise<T>) => work()
  };
  vi.spyOn(server, 'withTyping');
  const agent = {
    reply: vi.fn<Parameters<typeof createTurnHandler>[3]['reply']>().mockResolvedValue('One answer')
  };
  const controller = new AbortController();
  const handle = createTurnHandler(
    settings,
    'bot',
    server,
    agent,
    controller.signal,
    beforeEffects
  );
  const ctx = createTaskContext();
  const turn = (messages: readonly IncomingMessage[]) => handle(ctx, messages);
  const one: IncomingMessage = {
    id: 'one',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: 'First',
    reasons: ['mention']
  };
  const two: IncomingMessage = { ...one, id: 'two', authorId: 'another', body: 'Clarification' };
  return { ...fixture, settings, server, agent, controller, turn, one, two };
}

test.each([true, false])(
  'finishes pending typing before the final post, independently of naming (addressed: %s)',
  async (addressed) => {
    const { turn, one, server, agent } = setup();
    const finishModel = Promise.withResolvers<string>();
    const finishNaming = Promise.withResolvers<void>();
    const refresh = Promise.withResolvers<void>();
    const events: string[] = [];
    const update = vi.fn(async () => {
      await refresh.promise;
      events.push('typing');
    });
    vi.mocked(server.withTyping).mockImplementation((_destination, work) =>
      withTyping(new AbortController().signal, update, work)
    );
    naming.mockReturnValueOnce(finishNaming.promise);
    agent.reply.mockReturnValueOnce(finishModel.promise);
    server.postMessage.mockImplementationOnce(async () => {
      events.push('answer');
      return { ids: ['reply'] };
    });
    const pending = turn([{ ...one, reasons: addressed ? ['mention'] : [] }]);
    await vi.waitFor(() => expect(agent.reply).toHaveBeenCalledOnce());
    finishModel.resolve('Done');
    await vi.advanceTimersByTimeAsync(0);
    expect(server.postMessage).not.toHaveBeenCalled();
    refresh.resolve();
    await vi.waitFor(() => expect(server.postMessage).toHaveBeenCalledOnce());
    await vi.advanceTimersByTimeAsync(10_000);
    expect(events).toEqual(['typing', 'answer']);
    expect(update).toHaveBeenCalledOnce();
    finishNaming.resolve();
    await pending;
  }
);

test('posts the reply while naming is pending and joins naming before the turn ends', async () => {
  const { turn, one, history, server } = setup();
  history.mockResolvedValue(page([row('root', 'Root'), row('one', 'First')]));
  let finish!: () => void;
  naming.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      })
  );
  let ended = false;
  const pending = turn([one]).then(() => {
    ended = true;
  });
  await vi.waitFor(() => expect(server.postMessage).toHaveBeenCalledOnce());
  expect(ended).toBe(false);
  expect(naming.mock.calls[0]![1]).toBe('First');
  finish();
  await pending;
  expect(ended).toBe(true);
});

test('passive silence completes without typing, acknowledgement, or a post', async () => {
  const { turn, one, server, agent } = setup();
  participation.mockResolvedValueOnce({ action: 'ignore' });
  await turn([{ ...one, reasons: [] }]);
  expect(agent.reply).not.toHaveBeenCalled();
  expect(naming).not.toHaveBeenCalled();
  expect(server.withTyping).not.toHaveBeenCalled();
  expect(server.addReaction).not.toHaveBeenCalled();
  expect(server.postMessage).not.toHaveBeenCalled();
});

test('a passive reaction can be the entire response', async () => {
  const { turn, one, server, agent } = setup();
  participation.mockResolvedValueOnce({ action: 'react', emoji: 'tada' });
  await turn([{ ...one, reasons: [] }]);
  expect(server.addReaction).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', messageId: 'one' },
    'tada',
    { signal: expect.any(AbortSignal) }
  );
  expect(server.withTyping).not.toHaveBeenCalled();
  expect(server.postMessage).not.toHaveBeenCalled();
  expect(agent.reply).not.toHaveBeenCalled();
  expect(naming).not.toHaveBeenCalled();
});

test.each(['reply', 'ignore'] as const)(
  'reaction activity uses the classifier and delivers %s without reading the event as a message',
  async (action) => {
    const { turn, one, server, agent, history } = setup();
    history.mockResolvedValue(page([row('root', 'Root'), row('answer', 'Bot answer')]));
    participation.mockResolvedValueOnce({ action });
    const reaction = {
      ...one,
      id: 'reaction-event',
      body: '',
      reasons: [],
      reaction: { messageId: 'answer', emoji: 'thinking_face' }
    };
    await turn([reaction]);
    expect(participation).toHaveBeenCalledOnce();
    expect(
      server.getMessage.mock.calls.some(([target]) => target.messageId === 'reaction-event')
    ).toBe(false);
    expect(server.addReaction).not.toHaveBeenCalled();
    if (action === 'reply') {
      expect(JSON.parse(agent.reply.mock.calls[0]![1].prompt)).toEqual({
        currentDateTime: new Date().toISOString(),
        currentMessages: [{ author: 'human', reaction: reaction.reaction }]
      });
      expect(server.postMessage).toHaveBeenCalledOnce();
      expect(server.postMessage.mock.calls[0]![0]).toEqual({
        roomId: 'room',
        threadRootId: 'root',
        inReplyTo: 'answer'
      });
    } else {
      expect(agent.reply).not.toHaveBeenCalled();
      expect(server.withTyping).not.toHaveBeenCalled();
    }
  }
);

test('a passive follow-up can still receive a textual answer', async () => {
  const { turn, one, server, agent } = setup();
  agent.reply.mockResolvedValueOnce('Thanks, that answers my question.');
  await turn([{ ...one, reasons: [] }]);
  expect(server.postMessage).toHaveBeenCalledOnce();
  expect(server.postMessage.mock.calls[0]![1]).toBe('Thanks, that answers my question.');
  expect(server.addReaction).not.toHaveBeenCalled();
  expect(server.withTyping).toHaveBeenCalledOnce();
});

test.each(['direct_message', 'mention', 'reply'] as const)(
  'addressed %s bypasses the classifier',
  async (reason) => {
    const { turn, one, server, agent } = setup();
    participation.mockRejectedValue(new Error('Must not classify'));
    await turn([{ ...one, reasons: [reason] }]);
    expect(participation).not.toHaveBeenCalled();
    expect(agent.reply).toHaveBeenCalledOnce();
    expect(server.withTyping).toHaveBeenCalledOnce();
  }
);

test('classification failure leaves the batch retryable and produces no effects', async () => {
  const beforeEffects = vi.fn();
  const { turn, one, server, agent } = setup(beforeEffects);
  participation.mockRejectedValueOnce(new Error('Classifier unavailable'));
  await expect(turn([{ ...one, reasons: [] }])).rejects.toThrow('Classifier unavailable');
  expect(beforeEffects).not.toHaveBeenCalled();
  expect(server.withTyping).not.toHaveBeenCalled();
  expect(server.postMessage).not.toHaveBeenCalled();
  expect(server.addReaction).not.toHaveBeenCalled();
  expect(agent.reply).not.toHaveBeenCalled();
});

test('failed classified reactions are not retried or replaced with a textual answer', async () => {
  const beforeEffects = vi.fn();
  const { turn, one, server, agent } = setup(beforeEffects);
  participation.mockResolvedValueOnce({ action: 'react', emoji: '+1' });
  server.addReaction.mockRejectedValueOnce(new Error('Uncertain write'));
  await expect(turn([{ ...one, reasons: [] }])).rejects.toThrow('Uncertain write');
  expect(beforeEffects).toHaveBeenCalledOnce();
  expect(server.addReaction).toHaveBeenCalledOnce();
  expect(agent.reply).not.toHaveBeenCalled();
  expect(server.postMessage).not.toHaveBeenCalled();
});

test('cancellation after classification prevents the selected reaction', async () => {
  const { turn, one, server, controller } = setup();
  participation.mockImplementationOnce(async () => {
    controller.abort();
    return { action: 'react', emoji: 'heart' };
  });
  await expect(turn([{ ...one, reasons: [] }])).rejects.toThrow();
  expect(server.addReaction).not.toHaveBeenCalled();
});

test('passive typing starts before a completed message and ends even when later work fails', async () => {
  const { turn, one, server, agent } = setup();
  const release = Promise.withResolvers<void>();
  const acknowledged = Promise.withResolvers<void>();
  let typingActive = false;
  vi.mocked(server.withTyping).mockImplementation(async (_destination, work) => {
    typingActive = true;
    try {
      return await work();
    } finally {
      typingActive = false;
    }
  });
  agent.reply.mockImplementationOnce(async (_ctx, { onText }) => {
    expect(server.withTyping).toHaveBeenCalledOnce();
    expect(typingActive).toBe(true);
    expect(server.postMessage).not.toHaveBeenCalled();
    onText!("I'll check the forecast.");
    acknowledged.resolve();
    await release.promise;
    throw new Error('Model failed');
  });
  const pending = turn([{ ...one, reasons: [] }]);
  const rejected = expect(pending).rejects.toThrow('Model failed');
  await acknowledged.promise;
  expect(typingActive).toBe(true);
  release.resolve();
  await rejected;
  expect(typingActive).toBe(false);
  expect(server.postMessage).toHaveBeenCalledOnce();
});

test('a mixed batch keeps addressing per message and acknowledges the addressed message', async () => {
  const { turn, one, two, server, agent } = setup();
  await turn([{ ...one, reasons: [] }, two]);
  expect(
    JSON.parse(agent.reply.mock.calls[0]![1].prompt).currentMessages.map(
      (item: { reasons: string[] }) => item.reasons
    )
  ).toEqual([[], ['mention']]);
  expect(server.withTyping).toHaveBeenCalledOnce();
  expect(server.addReaction.mock.calls[0]![0].messageId).toBe('two');
  expect(participation).not.toHaveBeenCalled();
});

test('joins naming even when posting the reply fails', async () => {
  const { turn, one, history, server } = setup();
  history.mockResolvedValue(page([row('root', 'Root'), row('one', 'First')]));
  let finish!: () => void;
  naming.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      })
  );
  server.postMessage.mockRejectedValueOnce(new Error('Post failed'));
  let ended = false;
  const pending = turn([one]).catch((error: unknown) => {
    ended = true;
    return error;
  });
  try {
    await vi.waitFor(() => expect(server.postMessage).toHaveBeenCalledOnce());
    await vi.advanceTimersByTimeAsync(0);
    expect(ended).toBe(false);
  } finally {
    finish();
  }
  expect(await pending).toBeInstanceOf(Error);
});

test('passes image-only message pixels as request context, not persisted prompt text', async () => {
  const { turn, one, history, server, agent } = setup();
  const image = row('one', '');
  if (image.event.kind !== 'messagePosted') throw new Error('Expected message');
  image.event.attachments = [
    { id: 'image', filename: 'image.png', contentType: 'image/png', width: 10, height: 10 }
  ];
  history.mockResolvedValue(page([row('root', 'Root'), image]));
  server.readAttachment.mockResolvedValue({
    filename: 'image.png',
    contentType: 'image/png',
    data: new Uint8Array([1, 2, 3])
  });
  await turn([{ ...one, body: '' }]);
  const input = agent.reply.mock.calls[0]![1];
  expect(server.readAttachment).not.toHaveBeenCalled();
  expect((await input.loadImages!()).images).toEqual([
    { messageId: 'one', attachmentId: 'image', mimeType: 'image/png', data: 'AQID' }
  ]);
  expect(input.prompt).not.toContain('AQID');
  expect(server.postMessage).toHaveBeenCalledOnce();
});

test.each([false, true])(
  'does not install an image loader for text or document-only context (document: %s)',
  async (document) => {
    const { turn, one, history, server, agent } = setup();
    const message = row('one', 'Text request');
    if (message.event.kind !== 'messagePosted') throw new Error('Expected message');
    if (document)
      message.event.attachments = [
        {
          id: 'document',
          filename: 'notes.pdf',
          contentType: 'application/pdf',
          width: 0,
          height: 0
        }
      ];
    history.mockResolvedValue(page([row('root', 'Root'), message]));
    await turn([one]);
    expect(agent.reply.mock.calls[0]![1].loadImages).toBeUndefined();
    expect(server.readAttachment).not.toHaveBeenCalled();
  }
);

test('a failed durable start prevents reactions, model calls, and reply posts', async () => {
  const beforeEffects = vi.fn(() => {
    throw new Error('Storage unavailable');
  });
  const { turn, one, server, agent } = setup(beforeEffects);
  await expect(turn([one])).rejects.toThrow('Storage unavailable');
  expect(beforeEffects).toHaveBeenCalledExactlyOnceWith([one]);
  expect(server.addReaction).not.toHaveBeenCalled();
  expect(agent.reply).not.toHaveBeenCalled();
  expect(server.postMessage).not.toHaveBeenCalled();
});

test('failed context reads leave the batch safe to retry', async () => {
  const beforeEffects = vi.fn();
  const { turn, one, history, agent } = setup(beforeEffects);
  history.mockRejectedValue(new Error('Offline'));
  await expect(turn([one])).rejects.toThrow();
  expect(beforeEffects).not.toHaveBeenCalled();
  expect(agent.reply).not.toHaveBeenCalled();
});

test.each(['root', 'outside'])('includes the reply target ID and author: %s', async (target) => {
  const { turn, one, server, agent } = setup();
  server.getMessage.mockResolvedValue({ id: target, roomId: 'room', authorId: 'human' });
  await turn([{ ...one, inReplyTo: target }]);
  expect(JSON.parse(agent.reply.mock.calls[0]![1].prompt).currentMessages[0].inReplyTo).toEqual({
    id: target,
    author: 'human'
  });
  expect(server.getMessage).toHaveBeenCalledTimes(target === 'root' ? 0 : 1);
});

test('an unavailable reply target has an unknown author', async () => {
  const { turn, one, agent } = setup();
  await turn([{ ...one, inReplyTo: 'missing' }]);
  expect(JSON.parse(agent.reply.mock.calls[0]![1].prompt).currentMessages[0].inReplyTo).toEqual({
    id: 'missing',
    author: null
  });
});

test('a failed reply target lookup leaves the batch safe to retry', async () => {
  const beforeEffects = vi.fn();
  const { turn, one, server, agent } = setup(beforeEffects);
  server.getMessage.mockRejectedValue(new Error('Offline'));
  await expect(turn([{ ...one, inReplyTo: 'outside' }])).rejects.toThrow('Offline');
  expect(beforeEffects).not.toHaveBeenCalled();
  expect(agent.reply).not.toHaveBeenCalled();
});

test('later turns use the live timeline without another history request', async () => {
  const { turn, one, two, store, history, server, agent, controller } = setup();
  await turn([one]);
  vi.mocked(hasTranscript).mockResolvedValue(true);
  store().ingestEvent(row('unaddressed', 'Live context', 'another'));
  await turn([two]);
  expect(history).toHaveBeenCalledOnce();
  expect(agent.reply.mock.calls[1]?.[1].context?.messages).toContainEqual({
    id: 'unaddressed',
    author: 'another',
    fromViewer: false,
    text: 'Live context'
  });
  expect(server.postMessage).toHaveBeenCalledTimes(2);
  expect(server.addReaction).toHaveBeenCalledOnce();
  controller.abort();
});

test('each turn receives a fresh UTC date and time, including across midnight', async () => {
  const { turn, one, two, agent } = setup();
  vi.setSystemTime(new Date('2026-10-28T23:59:00.000Z'));
  await turn([one]);
  vi.setSystemTime(new Date('2026-10-29T00:01:00.000Z'));
  await turn([two]);
  expect(
    agent.reply.mock.calls.map(([, input]) => JSON.parse(input.prompt).currentDateTime)
  ).toEqual(['2026-10-28T23:59:00.000Z', '2026-10-29T00:01:00.000Z']);
});

test('a debounced batch becomes one prompt and one reply in its original thread', async () => {
  const { settings, server, agent, controller, turn, one, two, history } = setup();
  const failed = vi.fn();
  const inbox = new Inbox(turn, failed, settings);
  inbox.accept(one);
  await vi.advanceTimersByTimeAsync(1000);
  inbox.accept(two);
  await vi.advanceTimersByTimeAsync(5000);
  expect(failed).not.toHaveBeenCalled();
  expect(agent.reply).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ signal: expect.any(AbortSignal) }),
    {
      identity: ['https://chat.example.com', 'bot', 'room', 'root'],
      prompt: expect.any(String),
      signal: expect.any(AbortSignal),
      onText: expect.any(Function),
      loadImages: undefined,
      context: {
        messages: [
          { id: 'root', author: 'human', text: 'Root', fromViewer: false },
          {
            id: 'ordinary',
            author: 'human',
            text: 'Useful unaddressed context',
            fromViewer: false
          },
          { id: 'one', author: 'human', text: 'First', fromViewer: false },
          { id: 'two', author: 'another', text: 'Clarification', fromViewer: false }
        ],
        olderContextOmitted: false,
        imageNotes: []
      },
      userIds: ['human', 'another'],
      tools: [expect.any(Function), expect.any(Function), expect.any(Function)],
      capabilities: ['delegation', 'images', 'reactions']
    }
  );
  expect(JSON.parse(agent.reply.mock.calls[0]![1].prompt)).toEqual({
    currentDateTime: expect.any(String),
    currentMessages: [
      { id: 'one', author: 'human', text: 'First', reasons: ['mention'] },
      { id: 'two', author: 'another', text: 'Clarification', reasons: ['mention'] }
    ]
  });
  expect(history).toHaveBeenCalledOnce();
  expect(server.addReaction).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', messageId: 'one' },
    'eyes',
    { signal: expect.any(AbortSignal) }
  );
  expect(server.addReaction.mock.invocationCallOrder[0]).toBeLessThan(
    agent.reply.mock.invocationCallOrder[0]!
  );
  expect(server.postMessage).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', threadRootId: 'root', inReplyTo: 'two' },
    'One answer',
    { signal: expect.any(AbortSignal) }
  );
  controller.abort();
  await inbox.close();
});

test('failed acknowledgement still answers and acknowledgement can be disabled', async () => {
  const { settings, server, agent, turn, one } = setup();
  server.addReaction.mockRejectedValueOnce(new Error('unavailable'));
  await turn([one]);
  expect(server.postMessage).toHaveBeenCalledOnce();
  settings.acknowledgementEmoji = '';
  await turn([one]);
  expect(server.addReaction).toHaveBeenCalledOnce();
  expect(agent.reply).toHaveBeenCalledTimes(2);
});

test('a stalled acknowledgement times out without stopping the answer', async () => {
  const { server, agent, turn, one } = setup();
  server.addReaction.mockImplementationOnce(
    (_message, _emoji, options) =>
      new Promise((_, reject) => {
        options!.signal!.addEventListener('abort', () => reject(options!.signal!.reason), {
          once: true
        });
      })
  );
  // Native AbortSignal.timeout uses real timers, so inject the acknowledgement deadline.
  const deadline = new AbortController();
  const timeout = AbortSignal.timeout;
  vi.spyOn(AbortSignal, 'timeout').mockImplementation((ms) =>
    ms === 2000 ? deadline.signal : timeout(ms)
  );
  const pending = turn([one]);
  await vi.advanceTimersByTimeAsync(0);
  expect(agent.reply).not.toHaveBeenCalled();
  deadline.abort(new Error('deadline'));
  await pending;
  expect(server.postMessage).toHaveBeenCalledOnce();
});

test('a resumed thread does not get another acknowledgement', async () => {
  const { turn, one, server, settings } = setup();
  vi.mocked(hasTranscript).mockResolvedValue(true);
  await turn([one]);
  expect(hasTranscript).toHaveBeenCalledWith(settings.directory, [
    'https://chat.example.com',
    'bot',
    'room',
    'root'
  ]);
  expect(server.addReaction).not.toHaveBeenCalled();
  expect(server.postMessage).toHaveBeenCalledOnce();
});

test('failed or cancelled model work never posts a reply', async () => {
  const { server, agent, controller, turn, one } = setup();
  agent.reply.mockRejectedValueOnce(new Error('model failure'));
  await expect(turn([one])).rejects.toThrow('model failure');
  expect(server.postMessage).not.toHaveBeenCalled();
  agent.reply.mockImplementationOnce(async () => {
    controller.abort();
    return 'Too late';
  });
  await expect(turn([one])).rejects.toThrow();
  expect(server.postMessage).not.toHaveBeenCalled();
});

test('posts interim text before model completion, preserves order, and sends the final answer once', async () => {
  const { server, agent, turn, one } = setup();
  const firstPost = Promise.withResolvers<void>();
  const releasePost = Promise.withResolvers<void>();
  const finishModel = Promise.withResolvers<void>();
  server.postMessage.mockImplementationOnce(async () => {
    firstPost.resolve();
    await releasePost.promise;
    return { ids: ['interim'] };
  });
  agent.reply.mockImplementationOnce(async (_ctx, { onText }) => {
    onText!('Let me take a closer look.');
    await finishModel.promise;
    return 'Here is the answer.';
  });
  const pending = turn([one]);
  await firstPost.promise;
  expect(server.postMessage).toHaveBeenCalledTimes(1);
  releasePost.resolve();
  finishModel.resolve();
  await pending;
  expect(server.postMessage.mock.calls.map((call) => call[1])).toEqual([
    'Let me take a closer look.',
    'Here is the answer.'
  ]);
});

test('a failed interim post cancels model work and is never retried', async () => {
  const { server, agent, turn, one } = setup();
  server.postMessage.mockRejectedValueOnce(new Error('Uncertain write'));
  agent.reply.mockImplementationOnce(async (_ctx, { onText, signal }) => {
    const stopped = new Promise<never>((_resolve, reject) =>
      signal.addEventListener('abort', () => reject(signal.reason), { once: true })
    );
    onText!('Looking now.');
    onText!('Queued update.');
    return stopped;
  });
  await expect(turn([one])).rejects.toThrow();
  expect(server.postMessage).toHaveBeenCalledTimes(1);
});

test('an already delivered interim message survives a later model failure without another post', async () => {
  const { server, agent, turn, one } = setup();
  const posted = Promise.withResolvers<void>();
  server.postMessage.mockImplementationOnce(async () => {
    posted.resolve();
    return { ids: ['interim'] };
  });
  agent.reply.mockImplementationOnce(async (_ctx, { onText }) => {
    onText!('Looking now.');
    await posted.promise;
    throw new Error('Model failed');
  });
  await expect(turn([one])).rejects.toThrow('Model failed');
  expect(server.postMessage).toHaveBeenCalledTimes(1);
});
