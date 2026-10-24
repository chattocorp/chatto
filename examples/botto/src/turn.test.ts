/** Exercise inbox, shared client timeline, Pi input, and reply routing together. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { AddressedMessage, Destination, Server } from '@chatto/client';
import { Inbox } from './inbox.ts';
import { createTurnHandler } from './turn.ts';
import { parseSettings } from './settings.ts';
import { contextFixture, row } from './context.fixture.ts';
import { hasTranscript } from './messages.ts';

vi.mock('./messages.ts', () => ({ hasTranscript: vi.fn() }));

beforeEach(() => {
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

function setup(beforeEffects: (messages: readonly AddressedMessage[]) => void = () => {}) {
  const settings = parseSettings(
    '[chatto]\nurl="https://chat.example.com"\napi_key="test"\n[agent]\nmodel="test/model"',
    'botto.toml',
    {}
  );
  const fixture = contextFixture();
  fixtures.push(fixture.rooms);
  const server = {
    ...fixture.server,
    addReaction: vi.fn<Server['addReaction']>().mockResolvedValue(undefined),
    postMessage: vi.fn<Server['postMessage']>().mockResolvedValue({ ids: ['reply'] }),
    withTyping: async <T>(_destination: Destination, work: () => Promise<T>) => work()
  };
  const agent = {
    reply: vi.fn<Parameters<typeof createTurnHandler>[3]['reply']>().mockResolvedValue('One answer')
  };
  const controller = new AbortController();
  const turn = createTurnHandler(settings, 'bot', server, agent, controller.signal, beforeEffects);
  const one: AddressedMessage = {
    id: 'one',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: 'First',
    reasons: ['mention']
  };
  const two: AddressedMessage = { ...one, id: 'two', authorId: 'another', body: 'Clarification' };
  return { ...fixture, settings, server, agent, controller, turn, one, two };
}

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

test('later turns use the live timeline without another history request', async () => {
  const { turn, one, two, store, history, server, agent, controller } = setup();
  await turn([one]);
  vi.mocked(hasTranscript).mockResolvedValue(true);
  store().ingestEvent(row('unaddressed', 'Live context', 'another'));
  await turn([two]);
  expect(history).toHaveBeenCalledOnce();
  expect(agent.reply.mock.calls[1]?.[3]?.messages).toContainEqual({
    id: 'unaddressed',
    author: 'another',
    fromViewer: false,
    text: 'Live context'
  });
  expect(server.postMessage).toHaveBeenCalledTimes(2);
  expect(server.addReaction).toHaveBeenCalledOnce();
  controller.abort();
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
    ['https://chat.example.com', 'bot', 'room', 'root'],
    JSON.stringify({
      currentMessages: [
        { id: 'one', author: 'human', text: 'First' },
        { id: 'two', author: 'another', text: 'Clarification' }
      ]
    }),
    expect.any(AbortSignal),
    {
      messages: [
        { id: 'root', author: 'human', text: 'Root', fromViewer: false },
        { id: 'ordinary', author: 'human', text: 'Useful unaddressed context', fromViewer: false },
        { id: 'one', author: 'human', text: 'First', fromViewer: false },
        { id: 'two', author: 'another', text: 'Clarification', fromViewer: false }
      ],
      olderContextOmitted: false
    },
    ['human', 'another'],
    [expect.any(Function)]
  );
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
