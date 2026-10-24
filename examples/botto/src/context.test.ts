/** Exercise Botto's context selection from a thread read. */
import { expect, test } from 'vitest';
import { readContext } from './context.ts';
import { contextFixture, row } from './context.fixture.ts';

const location = { roomId: 'room', threadRootId: 'root' };

test('reads the requested window and keeps messages with text', async () => {
  const { server, thread, readThread } = contextFixture();
  thread.messages.push({ ...row('file', ''), attachments: [] }, row('reply', 'Mine', 'bot'));
  thread.olderOmitted = true;
  const signal = new AbortController().signal;
  const context = await readContext(server, location, 2, signal);
  expect(readThread).toHaveBeenCalledExactlyOnceWith(location, { limit: 2, signal });
  expect(context.messages.map((message) => message.id)).toEqual([
    'root',
    'ordinary',
    'one',
    'two',
    'reply'
  ]);
  expect(context.messages.at(-1)).toEqual({
    id: 'reply',
    author: 'bot',
    text: 'Mine',
    fromViewer: true
  });
  expect(context.olderContextOmitted).toBe(true);
});

test('rejects when the thread read fails', async () => {
  const { server, readThread } = contextFixture();
  readThread.mockRejectedValueOnce(new Error('Offline'));
  await expect(readContext(server, location, 2, new AbortController().signal)).rejects.toThrow(
    'Offline'
  );
});
