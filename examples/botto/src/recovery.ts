/** Recover retained notification sources using current authorized message content. */
import {
  DEFAULT_ADDRESSING_REASONS,
  type AddressedMessage,
  type AddressingReason,
  type Server,
  type ThreadLocation
} from '@chatto/client';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import type { NotificationSignal } from '@chatto/api-types/api/v1/notifications_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Delivery } from './delivery.ts';
import { isHumanAuthor } from './messages.ts';

/** The addressing reason of each notification kind that recovery handles; null is passive. */
const NOTIFICATION_REASONS: Partial<
  Record<NonNullable<NotificationSignal['kind']['case']>, AddressingReason | null>
> = {
  directMessageReceived: 'direct_message',
  directMentionReceived: 'mention',
  roleMentionReceived: 'role_mention',
  hereMentionReceived: 'here_mention',
  allMentionReceived: 'all_mention',
  replyReceived: 'reply',
  followedThreadActivity: null
};

/** Scan newest-first pages, then admit messages oldest-first. Never acknowledge or
 * dismiss notifications: the bot account can also be open in a human client. */
export async function recover(
  server: Pick<Server, 'service' | 'getMessage' | 'accountId'>,
  delivery: Delivery,
  isActive: (location: ThreadLocation) => Promise<boolean>,
  signal: AbortSignal,
  allowedUserId?: string,
  respondTo: readonly AddressingReason[] = DEFAULT_ADDRESSING_REASONS
): Promise<void> {
  const notifications = server.service(NotificationService);
  const candidates: { roomId: string; id: string; reasons: AddressedMessage['reasons'] }[] = [];
  let offset = 0;
  for (;;) {
    const page = await notifications.listNotificationOccurrences(
      { page: { limit: 100, offset } },
      { signal, timeoutMs: 10_000 }
    );
    let reachedStart = false;
    for (const item of page.occurrences) {
      const time = item.createdAt?.toDate().getTime();
      if (time === undefined) continue;
      if (time < delivery.since) {
        reachedStart = true;
        break;
      }
      const kind = item.signal?.kind;
      if (!kind || !kind.value || !('message' in kind.value)) continue;
      const reference = kind.value.message;
      if (!reference?.room?.id || !reference.eventId || delivery.has(reference.eventId)) continue;
      const reason = NOTIFICATION_REASONS[kind.case];
      if (reason === undefined) continue;
      // A kind without an enabled reason counts as passive, as on the live path.
      const reasons = reason && respondTo.includes(reason) ? [reason] : [];
      candidates.push({ roomId: reference.room.id, id: reference.eventId, reasons });
    }
    if (reachedStart || !page.page?.hasMore) break;
    if (!page.occurrences.length) throw new Error('Notification pagination did not advance');
    offset += page.occurrences.length;
  }
  for (const candidate of candidates.reverse()) {
    signal.throwIfAborted();
    if (delivery.has(candidate.id)) continue;
    let message;
    try {
      message = await server.getMessage(
        { roomId: candidate.roomId, messageId: candidate.id },
        { signal }
      );
      // Apply the live reply rules to retained notifications too.
      if (
        message?.threadRootId &&
        message.inReplyTo &&
        !candidate.reasons.includes('direct_message')
      ) {
        if (!respondTo.includes('reply')) continue;
        const target = await server.getMessage(
          { roomId: message.roomId, messageId: message.inReplyTo },
          { signal }
        );
        if (
          target?.authorId !== server.accountId ||
          (target.threadRootId || target.id) !== message.threadRootId
        )
          continue;
      }
    } catch (error) {
      if (
        error instanceof ConnectError &&
        [Code.NotFound, Code.PermissionDenied].includes(error.code)
      )
        continue;
      throw error;
    }
    if (
      !message ||
      (!message.body?.trim() &&
        !message.attachments?.some((attachment) => attachment.contentType.startsWith('image/'))) ||
      message.authorId === server.accountId ||
      (allowedUserId && message.authorId !== allowedUserId)
    )
      continue;
    if (
      !candidate.reasons.length &&
      (!message.threadRootId ||
        !(await isActive({ roomId: message.roomId, threadRootId: message.threadRootId })))
    )
      continue;
    if (!(await isHumanAuthor(server, message.authorId, signal))) continue;
    delivery.accept({ ...message, body: message.body ?? '', reasons: candidate.reasons });
  }
  delivery.prune();
}
