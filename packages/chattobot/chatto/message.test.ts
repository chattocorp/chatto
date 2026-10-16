import { expect, test } from 'vitest';
import { incomingMessage, replyTo } from './message.ts';
import { intakeMessage } from './message.ts';
import { RealtimeEvent } from '@chatto/client';

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

test('only new room-level posts in selected rooms are intake candidates', () => {
  const event = () =>
    new RealtimeEvent({
      id: 'root',
      actorId: 'human',
      event: { case: 'messagePosted', value: { roomId: 'room', bodyPlaintext: '' } }
    });
  const rooms = new Set(['room']);
  expect(intakeMessage(event(), rooms, 'bot')).toMatchObject({
    id: 'root',
    threadRootId: 'root',
    body: '',
    trigger: 'intake'
  });
  expect(intakeMessage(event(), new Set(), 'bot')).toBeUndefined();
  expect(intakeMessage(event(), rooms, 'human')).toBeUndefined();
  for (const field of ['threadRootEventId', 'inReplyTo', 'echoOfEventId'] as const) {
    const next = event();
    if (next.event.case === 'messagePosted') next.event.value[field] = 'other';
    expect(intakeMessage(next, rooms, 'bot')).toBeUndefined();
  }
  const shredded = event();
  if (shredded.event.case === 'messagePosted') shredded.event.value.bodyPlaintext = undefined;
  expect(intakeMessage(shredded, rooms, 'bot')).toBeUndefined();
  expect(intakeMessage(new RealtimeEvent(), rooms, 'bot')).toBeUndefined();
});
