import { expect, test } from 'vitest';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { AddReactionRequest } from '@chatto/api-types/api/v1/reactions_pb';
import { fakeChatto } from './chatto/fake-chatto.ts';
import type { IncomingMessage } from './chatto/message.ts';
import { createEyesReaction, createHandledReaction } from './reaction.ts';

const message: IncomingMessage = {
  id: 'ping',
  roomId: 'room',
  threadRootId: 'root',
  authorId: 'alice',
  body: 'Hello'
};

test('reacts to the pinging message, not the thread root', async () => {
  const requests: AddReactionRequest[] = [];
  const { createApi } = fakeChatto({
    viewerId: 'bot',
    routes: (router) =>
      router.service(MessageService, {
        addReaction(request) {
          requests.push(request);
          return { added: true };
        }
      })
  });
  const api = createApi({ serverUrl: 'https://chat.example', apiKey: 'key' });
  await createEyesReaction(api)(message);

  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ roomId: 'room', messageEventId: 'ping', emoji: 'eyes' });
});

test('sets and clears the authenticated bot reaction on the thread root', async () => {
  const requests: { method: string; messageEventId: string; roomId: string; emoji: string }[] = [];
  const { createApi } = fakeChatto({
    viewerId: 'bot',
    routes: (router) =>
      router.service(MessageService, {
        addReaction(request) {
          requests.push({ method: 'add', ...request });
          return { added: true };
        },
        removeReaction(request) {
          requests.push({ method: 'remove', ...request });
          return { removed: true };
        }
      })
  });
  const setHandled = createHandledReaction(
    createApi({ serverUrl: 'https://chat.example', apiKey: 'key' })
  );
  const signal = new AbortController().signal;
  await setHandled(message, true, signal);
  await setHandled(message, false, signal);
  expect(requests).toEqual([
    expect.objectContaining({
      method: 'add',
      roomId: 'room',
      messageEventId: 'root',
      emoji: 'white_check_mark'
    }),
    expect.objectContaining({
      method: 'remove',
      roomId: 'room',
      messageEventId: 'root',
      emoji: 'white_check_mark'
    })
  ]);
});
