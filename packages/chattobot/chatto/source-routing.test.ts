import { afterEach, expect, test, vi } from 'vitest';
import { RealtimeEvent, RoomKind, type ConsumeRealtimeOptions } from '@chatto/client';
import { dispatchRoute } from '../../runling/src/runtime/routing.ts';
import { chattoSource } from './realtime.ts';

const mocks = vi.hoisted(() => ({ consume: vi.fn() }));
vi.mock('@chatto/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@chatto/client')>();
  return {
    ...actual,
    createChattoClient: () => ({
      ...actual.createChattoClient({
        serverUrl: 'https://chat.example',
        apiKey: 'key',
        fetch: async () => Response.json({ user: { profile: { id: 'bot' } } })
      }),
      consumeRealtime: mocks.consume
    })
  };
});
afterEach(() => vi.unstubAllEnvs());

test('source retries failed registration through Runling dispatch and ignores accepted replay', async () => {
  vi.stubEnv('CHATTO_URL', 'https://chat.example');
  vi.stubEnv('CHATTO_API_KEY', 'key');
  const start = vi
    .fn()
    .mockRejectedValueOnce(new Error('Temporary journal failure'))
    .mockResolvedValue({ id: 'run' });
  const event = new RealtimeEvent({
    id: 'message',
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: { roomId: 'room', roomKind: RoomKind.DM, bodyPlaintext: 'hello' }
    }
  });
  mocks.consume.mockImplementation(async (options: ConsumeRealtimeOptions) => {
    await options.onEvent(event);
    expect(start).toHaveBeenCalledTimes(2);
    await options.onEvent(event);
    expect(start).toHaveBeenCalledTimes(2);
  });
  await chattoSource({
    signal: new AbortController().signal,
    state: new Map(),
    dispatch: (route, input) => dispatchRoute(route, input, start)
  });
});
