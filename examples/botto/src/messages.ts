/** Botto's participation policy, separate from the client's addressing rules. */
import { stat } from 'node:fs/promises';
import { join } from 'node:path';
import {
  DEFAULT_ADDRESSING_REASONS,
  type AddressedMessage,
  type AddressingReason,
  type RealtimeEvent,
  type Server,
  type ThreadLocation
} from '@chatto/client';
import { transcriptName } from './agent.ts';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';

/** Queued conversation activity. Reaction IDs identify the event, not a message. */
export interface IncomingMessage extends AddressedMessage {
  reaction?: { messageId: string; emoji: string };
}

/** Check current author state for live and recovered messages. Read failures
 * reject so delivery can retry; other bots must never trigger reply loops. */
export async function isHumanAuthor(
  server: Pick<Server, 'service'>,
  authorId: string,
  signal: AbortSignal
): Promise<boolean> {
  const { users } = await server
    .service(UserService)
    .batchGetUsers({ userIds: [authorId] }, { signal, timeoutMs: 10_000 });
  const author = users.find((item) => item.user?.id === authorId)?.user;
  return !!author && !author.bot && !author.deleted;
}

/** A saved Pi transcript keeps a thread active across restarts. No message cache is kept. */
export async function hasTranscript(
  directory: string,
  identity: readonly string[]
): Promise<boolean> {
  try {
    return (await stat(join(directory, transcriptName(identity)))).isFile();
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT')
      return false;
    throw error;
  }
}

/** Accept verified addressing for one of the `respondTo` reasons, or text and images
 * in a thread Botto already participates in. Passive messages use an empty reasons
 * list. The caller still checks the author and enforces its allowlist before
 * enqueueing; self messages never qualify. */
export async function incomingMessage(
  server: Pick<Server, 'accountId' | 'addressedMessage' | 'getMessage'>,
  event: RealtimeEvent,
  isActive: (location: ThreadLocation) => Promise<boolean>,
  signal: AbortSignal,
  respondTo: readonly AddressingReason[] = DEFAULT_ADDRESSING_REASONS
): Promise<IncomingMessage | undefined> {
  signal.throwIfAborted();
  if (!event.id || !event.actorId || event.actorId === server.accountId) return;
  if (event.event.case === 'reactionAdded') {
    const { roomId, messageEventId, emoji } = event.event.value;
    if (!roomId || !messageEventId || !emoji) return;
    const target = await server.getMessage({ roomId, messageId: messageEventId }, { signal });
    signal.throwIfAborted();
    if (!target || target.authorId !== server.accountId) return;
    return {
      id: event.id,
      roomId,
      threadRootId: target.threadRootId || target.id,
      authorId: event.actorId,
      body: '',
      reasons: [],
      reaction: { messageId: target.id, emoji }
    };
  }
  if (event.event.case !== 'messagePosted') return;
  const posted = event.event.value;
  if (!posted.roomId || posted.bodyPlaintext === undefined) return;
  // Realtime events contain no attachment metadata. Read only when text is absent.
  if (!posted.bodyPlaintext.trim()) {
    const message = await server.getMessage(
      { roomId: posted.roomId, messageId: event.id },
      { signal }
    );
    if (!message?.attachments?.some((attachment) => attachment.contentType.startsWith('image/')))
      return;
  }
  // DM messages address Botto even when they reply to an earlier human message.
  if (posted.roomKind === RoomKind.DM && respondTo.includes('direct_message'))
    return server.addressedMessage(event, { signal, reasons: ['direct_message'] });
  // In shared rooms, an explicit reply selects its recipient, even when it mentions the bot.
  if (posted.threadRootEventId && posted.inReplyTo)
    return respondTo.includes('reply')
      ? server.addressedMessage(event, { signal, reasons: ['reply'] })
      : undefined;
  const addressed = await server.addressedMessage(event, { signal, reasons: respondTo });
  if (addressed) return addressed;
  if (
    !posted.threadRootEventId ||
    !(await isActive({ roomId: posted.roomId, threadRootId: posted.threadRootEventId }))
  )
    return;
  signal.throwIfAborted();
  return {
    id: event.id,
    roomId: posted.roomId,
    threadRootId: posted.threadRootEventId,
    authorId: event.actorId,
    body: posted.bodyPlaintext,
    reasons: [],
    ...(posted.inReplyTo ? { inReplyTo: posted.inReplyTo } : {})
  };
}
