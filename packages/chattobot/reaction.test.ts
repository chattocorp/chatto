import { expect, test } from 'vitest';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { AddReactionRequest } from '@chatto/api-types/api/v1/reactions_pb';
import { fakeChatto } from './chatto/fake-chatto.ts';
import type { IncomingMessage } from './chatto/message.ts';
import { createEyesReaction } from './reaction.ts';

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
