/** Read a bounded thread snapshot for a quiet conversation's participation check. */
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import type { createApi } from '@chatto/client';
import type { Static } from 'runling';
import type { FollowUpInput } from '../tasks/classify-intake.ts';
import { threadOf, type IncomingMessage } from './message.ts';

export interface FollowUpSnapshot {
  /** The bot post that starts this batch. Message IDs are opaque, never sorted. */
  boundary: string;
  input: Omit<Static<typeof FollowUpInput>, 'messages'>;
  /** Keep each routable message with its classifier metadata when filtering replies. */
  messages: { message: IncomingMessage; attachmentTypes: string[] }[];
}

export type ReadFollowUp = (
  message: IncomingMessage,
  signal: AbortSignal
) => Promise<FollowUpSnapshot | undefined>;

/** Read only as the bot. Unknown, deleted, other-bot, and disallowed authors cannot engage it.
 * If the latest page has no bot post, do not guess a boundary across omitted history. */
export function createFollowUpReader(
  api: Pick<ReturnType<typeof createApi>, 'readThread' | 'service'>,
  allowedUserId?: string
): ReadFollowUp {
  return async (message, signal) => {
    const thread = await api.readThread(threadOf(message), { signal });
    const boundaryIndex = thread.messages.findLastIndex((item) => item.fromViewer);
    const boundary = thread.messages[boundaryIndex];
    if (!boundary || (boundaryIndex === 0 && thread.olderOmitted)) return;
    const candidates = thread.messages
      .slice(boundaryIndex + 1)
      .filter((item) => item.authorId && (!allowedUserId || item.authorId === allowedUserId));
    if (!candidates.length) return;
    const { users } = await api
      .service(UserService)
      .batchGetUsers(
        { userIds: [...new Set(candidates.map((item) => item.authorId!))] },
        { signal, timeoutMs: 10_000 }
      );
    const humans = new Set(
      users.flatMap(({ user }) => (user && !user.bot && !user.deleted ? [user.id] : []))
    );
    const replies = candidates.filter((item) => humans.has(item.authorId!));
    if (!replies.length) return;
    return {
      boundary: boundary.id,
      input: {
        ...(message.trigger === 'intake' ? { trackHandled: true } : {}),
        context: thread.messages
          .slice(Math.max(0, boundaryIndex - 3), boundaryIndex + 1)
          .map((item) => ({
            role: item.fromViewer ? ('bot' as const) : ('human' as const),
            body: item.body
          }))
      },
      messages: replies.map((item) => ({
        message: {
          ...threadOf(message),
          id: item.id,
          authorId: item.authorId!,
          body: item.body,
          trigger: 'follow-up' as const
        },
        attachmentTypes: (item.attachments ?? []).map((attachment) => attachment.contentType)
      }))
    };
  };
}
