import type { MessageLink } from '$lib/messageLinks';
import { parseMessageLink } from '$lib/messageLinks';
import { extractURLs } from '$lib/linkPreview';
import {
  isMessagePostedEvent,
  type MessagePostedPayload,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import type { RoomMember } from '$lib/state/room';

export type MessageEventReferences = {
  isEcho: boolean;
  editEventId: string;
  editThreadRootEventId: string | null;
  editChannelEchoEventId: string | null;
  threadRootEventId: string | null;
};

export type MessageReplyPreview = {
  name: string;
  body: string | null;
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
    return { name: missingName, body: null, actor: null, deleted: false };
  }

  const targetActor = target.actor ?? null;
  const activeActor = targetActor && !targetActor.deleted ? targetActor : null;

  return {
    name: activeActor ? getDisplayName(activeActor) : deletedName,
    body: isMessagePostedEvent(target.event) ? (target.event.body ?? null) : null,
    actor: activeActor,
    deleted: !activeActor
  };
}
