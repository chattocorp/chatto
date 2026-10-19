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
    read: (automatic = false, pending: string[] = []) =>
      createFollowUpReader(api, allowed)(
        { ...location, ...(automatic ? { trigger: 'intake' as const } : {}) },
        new AbortController().signal,
        pending
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

test('retains pending replies before the latest bot post with their earlier question as context', async () => {
  const h = harness([
    post('old-discussion'),
    post('question', 'bot', 'Which browser and version?'),
    post('one', 'human', 'Firefox'),
    post('two', 'human', '142'),
    post('answer', 'bot', 'I am still checking.'),
    post('three', 'human', 'On Linux')
  ]);
  const result = await h.read(false, ['one', 'two']);
  expect(result?.messages.map(({ message }) => message.id)).toEqual(['one', 'two', 'three']);
  expect(result?.input.context).toEqual([
    { role: 'human', body: 'old-discussion' },
    { role: 'bot', body: 'Which browser and version?' },
    { role: 'bot', body: 'I am still checking.' }
  ]);
});

test('pending IDs do not bypass author filters or restore missing messages from event bodies', async () => {
  const h = harness(
    [
      post('question', 'bot'),
      post('one', 'human'),
      post('two', 'maintainer'),
      post('three', 'deleted'),
      post('four', 'other-bot'),
      post('five', 'unknown'),
      post('answer', 'bot')
    ],
    'human'
  );
  expect(
    (await h.read(false, ['one', 'two', 'three', 'four', 'five', 'missing']))?.messages.map(
      ({ message }) => message.id
    )
  ).toEqual(['one']);
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
  expect(result?.messages.map(({ message }) => message)).toEqual([
    { ...location, id: 'a', body: '0.4.3', trigger: 'follow-up' },
    { ...location, id: 'b', authorId: 'maintainer', body: 'Docker Compose', trigger: 'follow-up' }
  ]);
  expect(result?.input.context.at(-1)).toEqual({ role: 'bot', body: 'latest-bot' });
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
  expect((await harness(messages).read())?.messages.map(({ message }) => message.id)).toEqual([
    'd',
    'e'
  ]);
  const filtered = harness(messages, 'human');
  expect((await filtered.read())?.messages.map(({ message }) => message.id)).toEqual(['d']);
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
  expect((await h.read())?.messages).toEqual([
    {
      message: { ...location, id: 'image', body: '', trigger: 'follow-up' },
      attachmentTypes: ['image/png']
    }
  ]);
});
