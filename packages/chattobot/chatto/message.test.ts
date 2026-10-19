import { expect, test, vi } from 'vitest';
import { incomingMessage, replyTo, reactionMessage } from './message.ts';
import { intakeMessage, followUpMessage } from './message.ts';
import { RealtimeEvent } from '@chatto/client';

test('reactions preserve the actor, event identity, exact target, and server-resolved thread', async () => {
  const event = new RealtimeEvent({
    id: 'reaction',
    actorId: 'maintainer',
    event: {
      case: 'reactionAdded',
      value: { roomId: 'room', messageEventId: 'proposal', emoji: '+1' }
    }
  });
  const getMessage = vi.fn().mockResolvedValue({
    id: 'proposal',
    authorId: 'bot',
    threadRootId: 'root',
    body: 'Shall I file this bug?'
  });
  const input = await reactionMessage(event, 'bot', { getMessage }, new AbortController().signal);
  expect(input).toEqual({
    id: 'reaction',
    authorId: 'maintainer',
    roomId: 'room',
    threadRootId: 'root',
    body: '+1',
    reaction: { emoji: '+1', messageId: 'proposal', body: 'Shall I file this bug?' }
  });
  expect(replyTo(input!)).toEqual({ roomId: 'room', threadRootId: 'root', inReplyTo: 'proposal' });
  getMessage.mockResolvedValue({ id: 'proposal', authorId: 'bot', body: 'Root proposal' });
  expect(
    await reactionMessage(event, 'bot', { getMessage }, new AbortController().signal)
  ).toMatchObject({ threadRootId: 'proposal' });
});

test('own reactions, removals, missing targets, and reactions to other people do not become input', async () => {
  const getMessage = vi.fn();
  const signal = new AbortController().signal;
  const event = new RealtimeEvent({
    id: 'reaction',
    actorId: 'bot',
    event: {
      case: 'reactionAdded',
      value: { roomId: 'room', messageEventId: 'proposal', emoji: 'heart' }
    }
  });
  expect(await reactionMessage(event, 'bot', { getMessage }, signal)).toBeUndefined();
  const removed = new RealtimeEvent({
    id: 'removed',
    actorId: 'human',
    event: {
      case: 'reactionRemoved',
      value: { roomId: 'room', messageEventId: 'proposal', emoji: 'heart' }
    }
  });
  expect(await reactionMessage(removed, 'bot', { getMessage }, signal)).toBeUndefined();
  expect(getMessage).not.toHaveBeenCalled();
  event.actorId = 'human';
  for (const target of [
    undefined,
    { id: 'proposal', authorId: 'human', body: 'Please approve' },
    { id: 'proposal', authorId: 'bot' }
  ]) {
    getMessage.mockResolvedValue(target);
    expect(await reactionMessage(event, 'bot', { getMessage }, signal)).toBeUndefined();
  }
});

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
