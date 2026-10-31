/** Exercise participation with the real client addressing rules, without a network. */
import { expect, test, vi } from 'vitest';
import { RealtimeEvent } from '@chatto/client';
import { type AddressingReason, MessagingRequests } from '@chatto/client/messaging/requests';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import {
  DirectUserMention,
  HereMessageMention,
  MessageMention
} from '@chatto/api-types/realtime/v1/events_pb';

import { incomingMessage } from './messages.ts';

/** A mention of Botto by name. */
const directMention = () =>
  new MessageMention({
    includesViewer: true,
    cause: { case: 'direct', value: new DirectUserMention({ userId: 'bot' }) }
  });

function setup() {
  const getMessage = vi.fn().mockResolvedValue({
    message: new Message({
      id: 'target',
      roomId: 'room',
      actorId: 'bot',
      threadRootEventId: 'root',
      body: 'Answer'
    })
  });
  const requests = new MessagingRequests(
    { service: vi.fn().mockReturnValue({ getMessage }) },
    async () => 'bot'
  );
  const server = {
    accountId: 'bot',
    addressedMessage: requests.addressedMessage.bind(requests),
    getMessage: requests.getMessage.bind(requests)
  };
  const active = vi.fn(async () => true);
  const event = new RealtimeEvent({
    id: 'incoming',
    actorId: 'human',
    event: {
      case: 'messagePosted',
      value: {
        roomId: 'room',
        roomKind: RoomKind.CHANNEL,
        bodyPlaintext: 'Follow-up',
        threadRootEventId: 'root'
      }
    }
  });
  return {
    event,
    active,
    getMessage,
    read: (signal = new AbortController().signal, respondTo?: readonly AddressingReason[]) =>
      incomingMessage(server, event, active, signal, respondTo)
  };
}

test('accepts reactions to Botto messages as passive activity in the target thread', async () => {
  const { event, read, active } = setup();
  event.event = new RealtimeEvent({
    event: {
      case: 'reactionAdded',
      value: {
        roomId: 'room',
        messageEventId: 'target',
        emoji: 'thinking_face'
      }
    }
  }).event;
  expect(await read()).toEqual({
    id: 'incoming',
    roomId: 'room',
    threadRootId: 'root',
    authorId: 'human',
    body: '',
    reasons: [],
    reaction: { messageId: 'target', emoji: 'thinking_face' }
  });
  expect(active).not.toHaveBeenCalled();
});

test('ignores own reactions, removals, and reactions to other authors', async () => {
  const { event, read, getMessage } = setup();
  event.event = new RealtimeEvent({
    event: {
      case: 'reactionAdded',
      value: {
        roomId: 'room',
        messageEventId: 'target',
        emoji: '+1'
      }
    }
  }).event;
  event.actorId = 'bot';
  expect(await read()).toBeUndefined();
  expect(getMessage).not.toHaveBeenCalled();
  event.actorId = 'human';
  getMessage.mockResolvedValue({
    message: new Message({ id: 'target', roomId: 'room', actorId: 'someone-else', body: 'Hi' })
  });
  expect(await read()).toBeUndefined();
  event.event = new RealtimeEvent({
    event: {
      case: 'reactionRemoved',
      value: {
        roomId: 'room',
        messageEventId: 'target',
        emoji: '+1'
      }
    }
  }).event;
  getMessage.mockClear();
  expect(await read()).toBeUndefined();
  expect(getMessage).not.toHaveBeenCalled();
});

test.each([RoomKind.DM, RoomKind.CHANNEL])(
  'accepts image-only messages in DMs and active threads (%s)',
  async (roomKind) => {
    const { event, getMessage, read } = setup();
    if (event.event.case !== 'messagePosted') throw new Error('Fixture');
    event.event.value.bodyPlaintext = '';
    event.event.value.roomKind = roomKind;
    getMessage.mockResolvedValue({
      message: new Message({
        id: event.id,
        roomId: 'room',
        actorId: 'human',
        attachments: [{ id: 'image', filename: 'image.png', contentType: 'image/png' }]
      })
    });
    expect(await read()).toMatchObject({ id: 'incoming', body: '' });
    getMessage.mockResolvedValue({
      message: new Message({ id: event.id, roomId: 'room', actorId: 'human' })
    });
    expect(await read()).toBeUndefined();
  }
);

test('ignores explicit thread replies when respond_to excludes reply', async () => {
  const { event, active, getMessage, read } = setup();
  if (event.event.case !== 'messagePosted') throw new Error('Fixture');
  event.event.value.inReplyTo = 'target';
  event.event.value.mentions.push(directMention());
  expect(await read(undefined, ['direct_message', 'mention'])).toBeUndefined();
  expect(getMessage).not.toHaveBeenCalled();
  expect(active).not.toHaveBeenCalled();
});

test.each(['', 'bot', 'human'])(
  'DM follow-ups stay addressed regardless of reply target: %s',
  async (targetAuthor) => {
    const { event, active, getMessage, read } = setup();
    if (event.event.case !== 'messagePosted') throw new Error('Fixture');
    event.event.value.roomKind = RoomKind.DM;
    if (targetAuthor) event.event.value.inReplyTo = 'target';
    getMessage.mockResolvedValue({
      message: new Message({ id: 'target', roomId: 'room', actorId: targetAuthor })
    });
    expect(await read(undefined, ['direct_message'])).toMatchObject({
      reasons: ['direct_message']
    });
    expect(active).not.toHaveBeenCalled();
    expect(getMessage).not.toHaveBeenCalled();
  }
);

test('answers @here only when respond_to enables it', async () => {
  const { event, active, read } = setup();
  if (event.event.case !== 'messagePosted') throw new Error('Fixture');
  event.event.value.threadRootEventId = '';
  event.event.value.mentions.push(
    new MessageMention({
      includesViewer: true,
      cause: { case: 'here', value: new HereMessageMention() }
    })
  );
  expect(await read()).toBeUndefined();
  expect((await read(undefined, ['mention', 'here_mention']))?.reasons).toEqual(['here_mention']);
  expect(active).not.toHaveBeenCalled();
});

test('ordinary text is accepted only in an active thread', async () => {
  const { active, read, getMessage } = setup();
  expect(await read()).toMatchObject({
    id: 'incoming',
    threadRootId: 'root',
    body: 'Follow-up',
    reasons: []
  });
  expect(active).toHaveBeenCalledWith({ roomId: 'room', threadRootId: 'root' });
  expect(getMessage).not.toHaveBeenCalled();
  active.mockResolvedValue(false);
  expect(await read()).toBeUndefined();
});

test.each(['mention', 'reply', 'direct_message'] as const)(
  'preserves client recognition for %s',
  async (reason) => {
    const { event, active, read } = setup();
    if (event.event.case !== 'messagePosted') throw new Error('Fixture');
    if (reason === 'mention') event.event.value.mentions.push(directMention());
    if (reason === 'reply') event.event.value.inReplyTo = 'target';
    if (reason === 'direct_message') event.event.value.roomKind = RoomKind.DM;
    active.mockResolvedValue(false);
    expect((await read())?.reasons).toEqual([reason]);
    expect(active).not.toHaveBeenCalled();
  }
);

test.each(['self', 'root', 'empty', 'other event'])(
  'ignores %s even with an active transcript',
  async (kind) => {
    const { event, active, read } = setup();
    if (event.event.case !== 'messagePosted') throw new Error('Fixture');
    if (kind === 'self') event.actorId = 'bot';
    if (kind === 'root') event.event.value.threadRootEventId = '';
    if (kind === 'empty') event.event.value.bodyPlaintext = '  ';
    if (kind === 'other event') event.event = { case: undefined };
    expect(await read()).toBeUndefined();
    expect(active).not.toHaveBeenCalled();
  }
);

test('does not enqueue after cancellation while checking participation', async () => {
  const { active, read } = setup();
  const controller = new AbortController();
  active.mockImplementation(async () => {
    controller.abort();
    return true;
  });
  await expect(read(controller.signal)).rejects.toThrow();
});

test.each([false, true])('ignores replies to another user (mention: %s)', async (mention) => {
  const { event, active, getMessage, read } = setup();
  if (event.event.case !== 'messagePosted') throw new Error('Fixture');
  event.event.value.inReplyTo = 'target';
  if (mention) event.event.value.mentions.push(directMention());
  getMessage.mockResolvedValue({
    message: new Message({
      id: 'target',
      roomId: 'room',
      actorId: 'other',
      threadRootEventId: 'root'
    })
  });
  expect(await read()).toBeUndefined();
  expect(active).not.toHaveBeenCalled();
});
