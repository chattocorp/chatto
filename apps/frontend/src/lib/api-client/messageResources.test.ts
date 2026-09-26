import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { createMessageResourcesAPI } from './messageResources';
import { REALTIME_MINIMUM_CURSOR_HEADER } from './connect';

const mocks = vi.hoisted(() => ({ messages: vi.fn(), users: vi.fn() }));
vi.mock('./connect', async (actual) => ({
  ...(await actual<typeof import('./connect')>()),
  createChattoClient: () => ({ batchGetMessages: mocks.messages })
}));
vi.mock('./users', () => ({ createUserAPI: () => ({ batchGetUsers: mocks.users }) }));

describe('shared message resource reads', () => {
  beforeEach(() => {
    mocks.messages.mockReset().mockResolvedValue({ messages: [] });
    mocks.users.mockReset().mockResolvedValue([]);
  });

  it('shares the raw message and normalized view and bounds user batches', async () => {
    const messages = Array.from(
      { length: 100 },
      (_, i) =>
        new Message({
          id: `M${i}`,
          roomId: 'R',
          actorId: `U${i}`,
          body: 'hello',
          thread: { participantPreviewUserIds: [`V${i}`] }
        })
    );
    mocks.messages.mockResolvedValue({ messages });
    const api = createMessageResourcesAPI({ baseUrl: 'http://localhost', bearerToken: null });
    const resources = await api.read(
      'R',
      messages.map((message) => message.id),
      'cursor'
    );
    expect(resources[0].message).toBe(messages[0]);
    expect(resources[0].timeline?.event).toMatchObject({ body: 'hello' });
    const [request, options] = mocks.messages.mock.calls[0];
    expect(request.eventIds).toHaveLength(100);
    expect(options.headers.get(REALTIME_MINIMUM_CURSOR_HEADER)).toBe('cursor');
    expect(options.headers.has('Authorization')).toBe(false);
    expect(mocks.users.mock.calls.map(([ids]) => ids.length)).toEqual([100, 100]);
    for (const [, cursor] of mocks.users.mock.calls) expect(cursor).toBe('cursor');
  });

  it('propagates user-read failure instead of committing incomplete reconciliation', async () => {
    mocks.messages.mockResolvedValue({ messages: [new Message({ id: 'M', actorId: 'U' })] });
    mocks.users.mockRejectedValue(new Error('users unavailable'));
    const api = createMessageResourcesAPI({ baseUrl: 'http://localhost', bearerToken: null });
    await expect(api.read('R', ['M'])).rejects.toThrow('users unavailable');
  });
});
