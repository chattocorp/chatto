import type { MessageLink } from '$lib/chatLinks';
import { parseMessageLink } from '$lib/chatLinks';
import { extractURLs } from '$lib/linkPreview';
import {
  isMessagePostedEvent,
  type MessagePostedPayload,
  type TimelineEventView
} from '@chatto/client/timeline/timelineEvents';
import type { RoomMember } from '$lib/state/room';
import type { UserAvatarUserView } from '@chatto/client/timeline/users';

/** The user profiles a message row resolves authors against, such as a `UserStore`. */
export type MessageAuthorProfiles = {
  view(id: string): UserAvatarUserView | undefined;
  isDeleted(id: string): boolean;
};

/** Who wrote a timeline event, as a message row shows it. */
export type MessageAuthor = {
  /** The profile to show, or `null` when the author is deleted or unknown. */
  user: UserAvatarUserView | null;
  /** Whether the author's account was deleted. */
  deleted: boolean;
};

/**
 * Resolve the author of a timeline event.
 *
 * A deleted account shows no profile, even when the event carried a copy of
 * it. Otherwise the live profile from the server's user store wins over the
 * copy that the event carried, which can be older.
 */
export function resolveMessageAuthor(
  event: TimelineEventView,
  users: MessageAuthorProfiles
): MessageAuthor {
  const id = event.actorId || event.actor?.id || '';
  const deleted =
    event.actorResolution === 'deleted' || !!event.actor?.deleted || users.isDeleted(id);
  if (deleted) return { user: null, deleted: true };
  return { user: users.view(id) ?? event.actor ?? null, deleted: false };
}

export type MessageEventReferences = {
  isEcho: boolean;
  editEventId: string;
  editThreadRootEventId: string | null;
  editChannelEchoEventId: string | null;
  threadRootEventId: string | null;
};

/** Attachment metadata for a reply label. No asset URLs are needed. */
export type MessageReplyAttachmentSummary = {
  kind: 'image' | 'video' | 'audio' | 'file' | 'attachments';
  count: number;
};

/** The author and content shown in a posted message's reply attribution. */
export type MessageReplyPreview = {
  name: string;
  /** Original text, or null when the body is empty or contains only whitespace. */
  body: string | null;
  /** Attachment label metadata when the target has no text. */
  attachmentSummary: MessageReplyAttachmentSummary | null;
  actor: RoomMember | null;
  deleted: boolean;
};

export function resolveMessageEventReferences(
  eventId: string,
  message: MessagePostedPayload
): MessageEventReferences {
  const isEcho = message.echoOfEventId != null;

  return {
    isEcho,
    editEventId: isEcho ? message.echoOfEventId! : eventId,
    editThreadRootEventId: isEcho
      ? (message.echoFromThreadRootEventId ?? null)
      : (message.threadRootEventId ?? null),
    editChannelEchoEventId: isEcho ? eventId : (message.channelEchoEventId ?? null),
    threadRootEventId: isEcho ? (message.echoFromThreadRootEventId ?? null) : eventId
  };
}

export function canEditMessage({
  isAuthor,
  createdAt,
  now,
  editWindowSeconds,
  canManageOthersMessage
}: {
  isAuthor: boolean;
  createdAt: string;
  now: number;
  editWindowSeconds: number;
  canManageOthersMessage: boolean;
}): boolean {
  return (
    (isAuthor && now - new Date(createdAt).getTime() < editWindowSeconds * 1000) ||
    canManageOthersMessage
  );
}

export function embeddedMessageLinks(body: string | null | undefined): MessageLink[] {
  if (!body) return [];

  return extractURLs(body, 5)
    .map(parseMessageLink)
    .filter((link): link is MessageLink => link !== null);
}

export function isDeletedMessage(message: MessagePostedPayload): boolean {
  return !message.body && message.attachments.length === 0;
}

/** Build a reply preview from the timeline's loaded target, without extra requests. */
export function buildMessageReplyPreview({
  target,
  missingName,
  deletedName,
  getDisplayName
}: {
  target: TimelineEventView | null | undefined;
  missingName: string;
  deletedName: string;
  getDisplayName: (member: RoomMember) => string;
}): MessageReplyPreview {
  if (!target) {
    return { name: missingName, body: null, attachmentSummary: null, actor: null, deleted: false };
  }

  const targetActor = target.actor ?? null;
  const activeActor = targetActor && !targetActor.deleted ? targetActor : null;
  const message = isMessagePostedEvent(target.event) ? target.event : null;
  const body = message?.body?.trim() ? message.body : null;
  let attachmentSummary: MessageReplyAttachmentSummary | null = null;
  if (!body && message && message.attachments.length > 0) {
    const count = message.attachments.length;
    const contentType = message.attachments[0].contentType;
    let kind: MessageReplyAttachmentSummary['kind'] = 'file';
    if (count > 1) kind = 'attachments';
    else if (contentType.startsWith('image/')) kind = 'image';
    else if (contentType.startsWith('video/')) kind = 'video';
    else if (contentType.startsWith('audio/')) kind = 'audio';
    attachmentSummary = { kind, count };
  }

  return {
    name: activeActor ? getDisplayName(activeActor) : deletedName,
    body,
    attachmentSummary,
    actor: activeActor,
    deleted: !activeActor
  };
}
