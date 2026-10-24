/** Botto's participation policy, separate from the client's addressing rules. */
import { stat } from 'node:fs/promises';
import { join } from 'node:path';
import type { AddressedMessage, RealtimeEvent, Server, ThreadLocation } from '@chatto/client';
import { transcriptName } from './agent.ts';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';

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

/** Accept verified addressing, or ordinary text in a thread Botto already participates in.
 * Passive messages use an empty reasons list. The caller still checks the author
 * and enforces its allowlist before enqueueing; self messages never qualify. */
export async function incomingMessage(
  server: Pick<Server, 'accountId' | 'addressedMessage'>,
  event: RealtimeEvent,
  isActive: (location: ThreadLocation) => Promise<boolean>,
  signal: AbortSignal
): Promise<AddressedMessage | undefined> {
  signal.throwIfAborted();
  if (
    event.event.case !== 'messagePosted' ||
    !event.id ||
    !event.actorId ||
    event.actorId === server.accountId
  )
    return;
  const posted = event.event.value;
  if (!posted.roomId || !posted.bodyPlaintext?.trim()) return;
  // An explicit reply selects its recipient, even when it also mentions the bot.
  if (posted.threadRootEventId && posted.inReplyTo)
    return server.addressedMessage(event, { signal, reasons: ['reply'] });
  const addressed = await server.addressedMessage(event, { signal });
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
