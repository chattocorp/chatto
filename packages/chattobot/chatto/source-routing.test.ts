import { afterEach, expect, test, vi } from 'vitest';
import { RealtimeEvent, RoomKind } from '@chatto/client';
import { dispatchRoute } from '../../runling/src/runtime/routing.ts';
import { chattoSource } from './realtime.ts';
import { fakeChatto, settle } from './fake-chatto.ts';

const chatto = fakeChatto({ viewerId: 'bot', routes: () => {} });
vi.mock('@chatto/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@chatto/client')>()),
  createClient: () => chatto.createClient(),
  createApi: (options: Parameters<typeof chatto.createApi>[0]) => chatto.createApi(options)
}));
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
  const controller = new AbortController();
  const running = chattoSource({
    signal: controller.signal,
    state: new Map(),
    dispatch: (route, input) => dispatchRoute(route, input, start)
  });
  await vi.waitFor(() => expect(chatto.connections.at(-1)?.listening).toBe(true));
  chatto.connections.at(-1)!.emit(event);
  await vi.waitFor(() => expect(start).toHaveBeenCalledTimes(2));
  chatto.connections.at(-1)!.emit(event);
  await settle();
  expect(start).toHaveBeenCalledTimes(2);
  controller.abort();
  await running;
});
