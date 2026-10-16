import { expect, test, vi } from 'vitest';
import type { ThreadMessage } from '@chatto/client';
import { createFollowUpReader } from './follow-up.ts';

const location = {
  id: 'root',
  roomId: 'room',
  threadRootId: 'root',
  authorId: 'human',
  body: 'Help'
};
const post = (id: string, authorId = 'human', body = id): ThreadMessage => ({
  id,
  authorId,
  body,
  fromViewer: authorId === 'bot'
});

function harness(messages: ThreadMessage[], allowed?: string) {
  const readThread = vi.fn(async () => ({ messages, olderOmitted: false }));
  const batchGetUsers = vi.fn(async () => ({
    users: [
      { user: { id: 'human' } },
      { user: { id: 'maintainer' } },
      { user: { id: 'other-bot', bot: {} } },
      { user: { id: 'deleted', deleted: true } }
    ]
  }));
  const api = { readThread, service: () => ({ batchGetUsers }) } as unknown as Parameters<
    typeof createFollowUpReader
  >[0];
  return {
    read: (automatic = false) =>
      createFollowUpReader(api, allowed)(
        { ...location, ...(automatic ? { trigger: 'intake' as const } : {}) },
        new AbortController().signal
      ),
    batchGetUsers,
    readThread
  };
}

test('only automatic-intake threads forward completion tracking to the classifier', async () => {
  const h = harness([post('root'), post('last', 'bot'), post('thanks', 'human', 'That fixed it!')]);
  expect((await h.read(true))?.input.trackHandled).toBe(true);
  expect((await h.read())?.input.trackHandled).toBeUndefined();
});

test('an old bot root cannot stand in for a boundary missing from the newest page', async () => {
  const h = harness([post('root', 'bot'), post('recent')]);
  h.readThread.mockResolvedValue({
    messages: [post('root', 'bot'), post('recent')],
    olderOmitted: true
  });
  expect(await h.read()).toBeUndefined();
  expect(h.batchGetUsers).not.toHaveBeenCalled();
});

test('uses the latest bot post as the boundary and preserves every eligible author in one batch', async () => {
  const h = harness([
    post('root'),
    post('old-bot', 'bot'),
    post('old-reply'),
    post('latest-bot', 'bot'),
    post('a', 'human', '0.4.3'),
    post('b', 'maintainer', 'Docker Compose')
  ]);
  const result = await h.read();
  expect(result?.boundary).toBe('latest-bot');
  expect(result?.messages).toEqual([
    { ...location, id: 'a', body: '0.4.3', trigger: 'follow-up' },
    { ...location, id: 'b', authorId: 'maintainer', body: 'Docker Compose', trigger: 'follow-up' }
  ]);
  expect(result?.input.context.at(-1)).toEqual({ role: 'bot', body: 'latest-bot' });
  expect(result?.input.messages).toEqual([
    { body: '0.4.3', attachmentTypes: [] },
    { body: 'Docker Compose', attachmentTypes: [] }
  ]);
});

test.each([[], [post('root')], [post('root'), post('last', 'bot')]])(
  'no snapshot is returned without a visible bot boundary and newer replies: %j',
  async (...messages) => {
    const h = harness(messages);
    expect(await h.read()).toBeUndefined();
    expect(h.batchGetUsers).not.toHaveBeenCalled();
  }
);

test('ignores other bots, deleted and unknown authors, and posts outside the allowed-user filter', async () => {
  const messages = [
    post('bot', 'bot'),
    post('a', 'other-bot'),
    post('b', 'deleted'),
    post('c', 'unknown'),
    post('d', 'human'),
    post('e', 'maintainer')
  ];
  expect((await harness(messages).read())?.messages.map((item) => item.id)).toEqual(['d', 'e']);
  const filtered = harness(messages, 'human');
  expect((await filtered.read())?.messages.map((item) => item.id)).toEqual(['d']);
  expect(filtered.batchGetUsers).toHaveBeenCalledWith({ userIds: ['human'] }, expect.anything());
});

test('attachment-only replies reach classification as MIME types without fetching their contents', async () => {
  const h = harness([
    post('bot', 'bot'),
    {
      ...post('image', 'human', ''),
      attachments: [{ id: 'attachment', filename: 'private.png', contentType: 'image/png' }]
    }
  ]);
  expect((await h.read())?.input.messages).toEqual([{ body: '', attachmentTypes: ['image/png'] }]);
});
