/** Exercise inbox, shared client timeline, Pi input, and reply routing together. */
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { AddressedMessage, Destination, Server } from '@chatto/client';
import { Inbox } from './inbox.ts';
import { createTurnHandler } from './turn.ts';
import { parseSettings } from './settings.ts';
import { contextFixture, row } from './context.fixture.ts';

beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(console, 'info').mockImplementation(() => {});
  vi.spyOn(console, 'error').mockImplementation(() => {});
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function setup() {
  const settings = parseSettings(
    '[chatto]\nurl="https://chat.example.com"\napi_key="test"\n[agent]\nmodel="test/model"',
    'botto.toml',
    {}
  );
  const fixture = contextFixture();
  const server = {
    ...fixture.server,
    postMessage: vi.fn<Server['postMessage']>().mockResolvedValue({ ids: ['reply'] }),
    withTyping: async <T>(_destination: Destination, work: () => Promise<T>) => work()
  };
  const agent = {
    reply: vi.fn<Parameters<typeof createTurnHandler>[3]['reply']>().mockResolvedValue('One answer')
  };
  const controller = new AbortController();
  const turn = createTurnHandler(settings, 'bot', server, agent, controller.signal);
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

test('later turns use the shared live timeline without another history request', async () => {
  const { turn, one, two, store, history, server, agent, controller } = setup();
  await turn([one]);
  store.ingestEvent(row('unaddressed', 'Live context', 'another'));
  await turn([two]);
  expect(history).toHaveBeenCalledTimes(1);
  expect(agent.reply.mock.calls[1]?.[3]?.messages).toContainEqual({
    id: 'unaddressed',
    author: 'another',
    fromViewer: false,
    text: 'Live context'
  });
  expect(server.postMessage).toHaveBeenCalledTimes(2);
  controller.abort();
  store.dispose();
});

test('a debounced batch becomes one prompt and one reply in its original thread', async () => {
  const { settings, server, agent, controller, turn, one, two, history, store } = setup();
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
    }
  );
  expect(history).toHaveBeenCalledTimes(1);
  expect(server.postMessage).toHaveBeenCalledExactlyOnceWith(
    { roomId: 'room', threadRootId: 'root', inReplyTo: 'two' },
    'One answer',
    { signal: expect.any(AbortSignal) }
  );
  controller.abort();
  await inbox.close();
  store.dispose();
});

test('failed or cancelled model work never posts a reply', async () => {
  const { server, agent, controller, turn, one, store } = setup();
  agent.reply.mockRejectedValueOnce(new Error('model failure'));
  await expect(turn([one])).rejects.toThrow('model failure');
  expect(server.postMessage).not.toHaveBeenCalled();
  agent.reply.mockImplementationOnce(async () => {
    controller.abort();
    return 'Too late';
  });
  await expect(turn([one])).rejects.toThrow();
  expect(server.postMessage).not.toHaveBeenCalled();
  store.dispose();
});
