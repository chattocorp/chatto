/** Recover retained notification sources using current authorized message content. */
import {
  DEFAULT_ADDRESSING_REASONS,
  type AddressedMessage,
  type AddressingReason,
  type Server,
  type ThreadLocation
} from '@chatto/client';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import { Code, ConnectError } from '@connectrpc/connect';
import type { Delivery } from './delivery.ts';
import { isHumanAuthor } from './messages.ts';

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
      let reasons: AddressedMessage['reasons'];
      switch (kind.case) {
        case 'directMessageReceived':
          reasons = ['direct_message'];
          break;
        case 'directMentionReceived':
          reasons = ['mention'];
          break;
        case 'roleMentionReceived':
          reasons = ['role_mention'];
          break;
        case 'hereMentionReceived':
          reasons = ['here_mention'];
          break;
        case 'allMentionReceived':
          reasons = ['all_mention'];
          break;
        case 'replyReceived':
          reasons = ['reply'];
          break;
        case 'followedThreadActivity':
          reasons = [];
          break;
        default:
          continue;
      }
      // Followed-thread activity is passive; other kinds need an enabled reason.
      if (reasons.length && !reasons.some((reason) => respondTo.includes(reason))) continue;
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
      // Apply the live reply-recipient rule to retained notifications too.
      if (message?.threadRootId && message.inReplyTo) {
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
      !message?.body?.trim() ||
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
    delivery.accept({ ...message, body: message.body, reasons: candidate.reasons });
  }
  delivery.prune();
}
