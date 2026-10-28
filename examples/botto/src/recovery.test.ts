/** Exercise offline catch-up against the generated notification and message types. */
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import type { Server } from '@chatto/client';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import {
  ListNotificationOccurrencesResponse,
  NotificationOccurrence
} from '@chatto/api-types/api/v1/notifications_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { Delivery } from './delivery.ts';
import { recover } from './recovery.ts';

const directories: string[] = [];
afterEach(() => {
  for (const path of directories.splice(0)) rmSync(path, { recursive: true, force: true });
});

function occurrence(
  id: string,
  time = 2000,
  kind: 'directMentionReceived' | 'hereMentionReceived' = 'directMentionReceived'
) {
  return new NotificationOccurrence({
    id: `notification-${id}`,
    createdAt: { seconds: BigInt(Math.floor(time / 1000)), nanos: (time % 1000) * 1_000_000 },
    signal: {
      kind: {
        case: kind,
        value: {
          message: { room: { id: 'room' }, eventId: id }
        }
      }
    }
  });
}

function setup() {
  const root = resolve('../../.context');
  mkdirSync(root, { recursive: true });
  const path = mkdtempSync(join(root, 'botto-recovery-'));
  directories.push(path);
  const delivery = new Delivery(path, ['bot'], 1000);
  const list = vi.fn().mockResolvedValue(
    new ListNotificationOccurrencesResponse({
      occurrences: [occurrence('two'), occurrence('one')]
    })
  );
  const getMessage = vi.fn(async ({ messageId }: { messageId: string }) => ({
    id: messageId,
    roomId: 'room',
    authorId: 'human',
    body: 'Current text'
  }));
  const users = vi.fn().mockResolvedValue({ users: [{ user: { id: 'human' } }] });
  // The generated service factory is generic; this fixture supplies only the two used services.
  const server = {
    accountId: 'bot',
    getMessage,
    service: (service: unknown) =>
      service === NotificationService
        ? { listNotificationOccurrences: list }
        : { batchGetUsers: users }
  } as unknown as Pick<Server, 'accountId' | 'getMessage' | 'service'>;
  const run = (
    target = delivery,
    allowedUserId?: string,
    respondTo?: Parameters<typeof recover>[5]
  ) =>
    recover(
      server,
      target,
      async () => false,
      new AbortController().signal,
      allowedUserId,
      respondTo
    );
  return { path, delivery, list, getMessage, users, run };
}

test('recovery admits image-only messages and skips empty messages', async () => {
  const { run, delivery, getMessage } = setup();
  getMessage.mockImplementation(async ({ messageId }) => ({
    id: messageId,
    roomId: 'room',
    authorId: 'human',
    body: '',
    ...(messageId === 'one'
      ? { attachments: [{ id: 'image', filename: 'image.png', contentType: 'image/png' }] }
      : {})
  }));
  await run();
  expect(delivery.queued.map((message) => message.id)).toEqual(['one']);
});

test('restart recovers paginated offline mentions in order and deduplicates live delivery', async () => {
  const { path, delivery, list, run } = setup();
  list
    .mockResolvedValueOnce(
      new ListNotificationOccurrencesResponse({
        occurrences: [occurrence('two')],
        page: { hasMore: true }
      })
    )
    .mockResolvedValueOnce(
      new ListNotificationOccurrencesResponse({
        occurrences: [occurrence('one'), occurrence('before-install', 999)]
      })
    );
  await run();
  expect(delivery.queued.map((item) => item.id)).toEqual(['one', 'two']);
  expect(list.mock.calls[1]?.[0].page.offset).toBe(1);
  delivery.complete(delivery.queued);
  const restarted = new Delivery(path, ['bot']);
  await run(restarted);
  expect(restarted.queued).toEqual([]);
});

test('a transient read failure is retried without losing older work', async () => {
  const { delivery, getMessage, run } = setup();
  getMessage.mockRejectedValueOnce(new ConnectError('private', Code.Unavailable));
  await expect(run()).rejects.toThrow();
  expect(delivery.since).toBe(1000);
  await run();
  expect(delivery.queued.map((item) => item.id)).toEqual(['one', 'two']);
});

test.each(['directMessageReceived', 'replyReceived'] as const)(
  'recovers offline %s notifications, including notifications already read',
  async (kind) => {
    const { delivery, list, run } = setup();
    list.mockResolvedValue(
      new ListNotificationOccurrencesResponse({
        occurrences: [
          new NotificationOccurrence({
            id: 'notification',
            createdAt: { seconds: 2n },
            unread: false,
            signal: {
              kind: { case: kind, value: { message: { room: { id: 'room' }, eventId: 'one' } } }
            }
          })
        ]
      })
    );
    await run();
    expect(delivery.queued[0]?.reasons).toEqual([
      kind === 'directMessageReceived' ? 'direct_message' : 'reply'
    ]);
  }
);

test('a later scan repairs a message omitted when live pagination changed', async () => {
  const { delivery, list, run } = setup();
  list.mockResolvedValueOnce(
    new ListNotificationOccurrencesResponse({ occurrences: [occurrence('two')] })
  );
  await run();
  delivery.complete(delivery.queued);
  await run();
  expect(delivery.queued.map((item) => item.id)).toEqual(['one']);
});

test('recovery ignores deleted messages, other bots, and disallowed authors', async () => {
  const { delivery, getMessage, users, run } = setup();
  getMessage.mockRejectedValueOnce(new ConnectError('private', Code.NotFound));
  users.mockResolvedValue({ users: [{ user: { id: 'human', bot: true } }] });
  await run();
  expect(delivery.queued).toEqual([]);
  users.mockResolvedValue({ users: [{ user: { id: 'human' } }] });
  await run(delivery, 'someone-else');
  expect(delivery.queued).toEqual([]);
});

test('recovered DM replies address Botto even when the reply target is another human message', async () => {
  const { run, delivery, list, getMessage } = setup();
  list.mockResolvedValue(
    new ListNotificationOccurrencesResponse({
      occurrences: [
        new NotificationOccurrence({
          id: 'notification-dm',
          createdAt: { seconds: 2n },
          signal: {
            kind: {
              case: 'directMessageReceived',
              value: { message: { room: { id: 'room' }, eventId: 'dm' } }
            }
          }
        })
      ]
    })
  );
  getMessage.mockImplementation(async () => ({
    id: 'dm',
    roomId: 'room',
    authorId: 'human',
    body: 'One more thing',
    threadRootId: 'root',
    inReplyTo: 'earlier-human-message'
  }));
  await run(delivery, undefined, ['direct_message']);
  expect(delivery.queued).toMatchObject([{ id: 'dm', reasons: ['direct_message'] }]);
  expect(getMessage).toHaveBeenCalledOnce();
});

test.each(['other', 'bot'])('recovery checks the reply recipient: %s', async (authorId) => {
  const { delivery, getMessage, run } = setup();
  getMessage.mockImplementation(async ({ messageId }) => ({
    id: messageId,
    roomId: 'room',
    authorId: messageId === 'target' ? authorId : 'human',
    body: 'Current text',
    threadRootId: 'root',
    ...(messageId !== 'target' ? { inReplyTo: 'target' } : {})
  }));
  await run();
  expect(delivery.queued.map((item) => item.id)).toEqual(authorId === 'bot' ? ['one', 'two'] : []);
});

test('recovers @here mentions only when respond_to enables them', async () => {
  const { delivery, list, getMessage, run } = setup();
  list.mockResolvedValue(
    new ListNotificationOccurrencesResponse({
      occurrences: [occurrence('here', 2000, 'hereMentionReceived')]
    })
  );
  // Without here_mention, the message counts as passive; Botto is not in the thread.
  await run();
  expect(delivery.has('here')).toBe(false);
  await run(delivery, undefined, ['mention', 'here_mention']);
  expect(delivery.has('here')).toBe(true);
  expect(getMessage).toHaveBeenCalledTimes(2);
});

test('skips recovered thread replies when respond_to excludes reply', async () => {
  const { delivery, getMessage, run } = setup();
  getMessage.mockImplementation(async ({ messageId }: { messageId: string }) => ({
    id: messageId,
    roomId: 'room',
    authorId: 'human',
    body: 'Reply text',
    threadRootId: 'root',
    inReplyTo: 'target'
  }));
  await run(delivery, undefined, ['direct_message', 'mention']);
  // Only the recovered messages are read; no reply target lookup follows.
  expect(getMessage).toHaveBeenCalledTimes(2);
  expect(delivery.has('one')).toBe(false);
  expect(delivery.has('two')).toBe(false);
});
