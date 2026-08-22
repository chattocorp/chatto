import { getEmojiByName } from '$lib/emoji';
import type { MessageActionParams, MessageActions } from '$lib/hooks';

type ReactionState = {
  emoji: string;
  hasReacted: boolean;
};

export type MessageActionModel = {
  serverId: string;
  messageBody: string;
  canReact: boolean;
  canEdit: boolean;
  canDelete: boolean;
  canPin: boolean;
  isPinned: boolean;
  /** Mode-aware standard Reply action. Required and Encouraged roots route it into a thread. */
  replyInRoomLabel: string;
  replyThreadLabel: string;
  replyInRoom?: () => void;
  replyThread?: () => void;
  /** Encouraged-mode fallback available in expanded action surfaces, not the quick toolbar. */
  secondaryReplyInRoomLabel?: string;
  secondaryReplyInRoom?: () => void;
  hasReacted: (emoji: string) => boolean;
  toggleReaction: (emoji: string) => Promise<void>;
  edit: () => void;
  copyText: () => Promise<void>;
  copyLink: () => Promise<void>;
  delete: () => void;
  togglePin: () => Promise<void>;
};

/** Binds one message target to the behavior shared by every message-action surface. */
export function buildMessageActionModel({
  actions,
  params,
  reactions,
  canReact,
  canEdit,
  canDelete,
  canPin,
  isPinned,
  togglePin,
  replyInRoomLabel,
  replyThreadLabel,
  replyInRoom,
  replyThread,
  secondaryReplyInRoomLabel,
  secondaryReplyInRoom
}: {
  actions: MessageActions;
  params: MessageActionParams;
  reactions: ReactionState[];
  canReact: boolean;
  canEdit: boolean;
  canDelete: boolean;
  canPin?: boolean;
  isPinned?: boolean;
  togglePin?: () => Promise<void>;
  replyInRoomLabel: string;
  replyThreadLabel: string;
  replyInRoom?: () => void;
  replyThread?: () => void;
  secondaryReplyInRoomLabel?: string;
  secondaryReplyInRoom?: () => void;
}): MessageActionModel {
  const viewerReactions = new Set(
    reactions
      .filter((reaction) => reaction.hasReacted)
      .map((reaction) => getEmojiByName(reaction.emoji) ?? reaction.emoji)
  );
  const hasReacted = (emoji: string) => viewerReactions.has(getEmojiByName(emoji) ?? emoji);

  return {
    serverId: params.serverId,
    messageBody: params.messageBody,
    canReact,
    canEdit,
    canDelete,
    canPin: canPin ?? false,
    isPinned: isPinned ?? false,
    replyInRoomLabel,
    replyThreadLabel,
    replyInRoom,
    replyThread,
    secondaryReplyInRoomLabel,
    secondaryReplyInRoom,
    hasReacted,
    toggleReaction: (emoji) => actions.toggleReaction(params, emoji, hasReacted(emoji)),
    edit: () => actions.startEdit(params),
    copyText: () => actions.copyMessageText(params),
    copyLink: () => actions.copyMessageLink(params),
    delete: () => actions.openDeleteConfirmation(params),
    togglePin: togglePin ?? (async () => {})
  };
}
