import { expect, test } from 'vitest';
import { incomingMessage, replyTo } from './message.ts';
import { intakeMessage, followUpMessage } from './message.ts';
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

test('follow-up notifications exclude roots, own posts, echoes, and shredded messages', () => {
  const event = new RealtimeEvent({
    id: 'reply',
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: { roomId: 'room', threadRootEventId: 'root', bodyPlaintext: '0.5' }
    }
  });
  expect(followUpMessage(event, 'bot')).toEqual({
    id: 'reply',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: '0.5',
    trigger: 'follow-up'
  });
  expect(followUpMessage(event, 'human')).toBeUndefined();
  if (event.event.case !== 'messagePosted') throw new Error('Fixture');
  event.event.value.echoOfEventId = 'echo';
  expect(followUpMessage(event, 'bot')).toBeUndefined();
  event.event.value.echoOfEventId = '';
  event.event.value.bodyPlaintext = undefined;
  expect(followUpMessage(event, 'bot')).toBeUndefined();
  event.event.value.bodyPlaintext = '';
  event.event.value.threadRootEventId = '';
  expect(followUpMessage(event, 'bot')).toBeUndefined();
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
