import { expect, test } from 'vitest';
import { incomingMessage, replyTo } from './message.ts';

test('a message at the room level starts its own thread, and a reply references the message', () => {
  const root = incomingMessage({
    id: 'one',
    roomId: 'room',
    authorId: 'alice',
    body: 'Hi',
    reasons: ['direct_message']
  });
  expect(root).toEqual({
    id: 'one',
    roomId: 'room',
    threadRootId: 'one',
    authorId: 'alice',
    body: 'Hi'
  });
  expect(replyTo({ ...root, id: 'two', threadRootId: 'root' })).toEqual({
    roomId: 'room',
    threadRootId: 'root',
    inReplyTo: 'two'
  });
});
