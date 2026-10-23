/** Read a bounded thread snapshot for a quiet conversation's participation check. */
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import type { createApi } from '@chatto/client';
import type { Static } from 'runling';
import type { FollowUpInput } from '../tasks/classify-intake.ts';
import { threadOf, type IncomingMessage } from './message.ts';

export interface FollowUpSnapshot {
  /** Latest bot post or conversation entry for reconciliation. Pending replies can predate it.
   * Message IDs are opaque, never sorted. */
  boundary: string;
  input: Omit<Static<typeof FollowUpInput>, 'messages'>;
  /** Keep each routable message with its classifier metadata when filtering replies. */
  messages: { message: IncomingMessage; attachmentTypes: string[] }[];
}

export type ReadFollowUp = (
  message: IncomingMessage,
  signal: AbortSignal,
  /** Observed replies that must remain candidates across a newer bot post. */
  pendingIds?: readonly string[]
) => Promise<FollowUpSnapshot | undefined>;

/** Read only as the bot. Unknown, deleted, other-bot, and disallowed authors cannot engage it.
 * Use the conversation entry until the bot posts text, including after reaction-only turns.
 * Do not guess a boundary across omitted history. */
export function createFollowUpReader(
  api: Pick<ReturnType<typeof createApi>, 'readThread' | 'service'>,
  allowedUserId?: string
): ReadFollowUp {
  return async (message, signal, pendingIds = []) => {
    const thread = await api.readThread(threadOf(message), { signal });
    const entryId = message.reaction?.messageId ?? message.id;
    const boundaryIndex = Math.max(
      thread.messages.findLastIndex((item) => item.fromViewer),
      thread.messages.findIndex((item) => item.id === entryId)
    );
    const boundary = thread.messages[boundaryIndex];
    if (!boundary || (boundaryIndex === 0 && thread.olderOmitted)) return;
    const pending = new Set(pendingIds);
    const candidates = thread.messages.filter(
      (item, index) =>
        (index > boundaryIndex || pending.has(item.id)) &&
        !item.fromViewer &&
        item.authorId &&
        (!allowedUserId || item.authorId === allowedUserId)
    );
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
    // Include the question an earlier pending reply answered, as well as newer bot posts.
    // Candidate replies belong only in `messages`, never a second time in `context`.
    const replyIds = new Set(replies.map((item) => item.id));
    const contextStart = Math.min(
      boundaryIndex,
      thread.messages.findIndex((item) => replyIds.has(item.id))
    );
    return {
      boundary: boundary.id,
      input: {
        ...(message.trigger === 'intake' ? { trackHandled: true } : {}),
        context: thread.messages
          .slice(Math.max(0, contextStart - 3), boundaryIndex + 1)
          .filter((item) => !replyIds.has(item.id))
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
