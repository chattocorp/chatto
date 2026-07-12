import { useConnection } from '$lib/state/server/connection.svelte';
import { toast } from '$lib/ui/toast';
import { pushState } from '$app/navigation';
import { getComposerContext, type MessagesStore } from '$lib/state/room';
import { emojiToName } from '$lib/emoji';
import { copyMessageLinkToClipboard } from '$lib/messageLinks';
import { createReactionAPI } from '$lib/api-client/reactions';
import * as m from '$lib/i18n/messages';

export type MessageActionParams = {
  serverId: string;
  roomId: string;
  messageEventId: string;
  eventId: string;
  deleteEventId?: string;
  messageBody: string;
  threadRootEventId?: string | null;
  channelEchoEventId?: string | null;
  canAddChannelEcho?: boolean;
  messageStore?: MessagesStore | null;
};

/** Shared reaction mutation handlers for all message reaction controls. */
export function useReactionActions() {
  const connection = useConnection();

  function reactionName(emojiOrName: string): string | null {
    return emojiToName(emojiOrName) ?? emojiOrName;
  }

  async function addReaction(params: MessageActionParams, emojiOrName: string) {
    const name = reactionName(emojiOrName);
    if (!name) return;
    const optimistic = params.messageStore?.beginOptimisticReaction({
      messageEventId: params.messageEventId,
      emoji: name,
      action: 'add'
    });

    try {
      const conn = connection();
      const result = await createReactionAPI({
        serverId: conn.serverId ?? params.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).addReaction({
        roomId: params.roomId,
        messageEventId: params.messageEventId,
        emoji: name
      });
      optimistic?.applyServerReaction(result.reaction);
    } catch {
      optimistic?.rollback();
      toast.error(m['room.message.reaction_failed']());
    }
  }

  async function removeReaction(params: MessageActionParams, emojiOrName: string) {
    const name = reactionName(emojiOrName);
    if (!name) return;
    const optimistic = params.messageStore?.beginOptimisticReaction({
      messageEventId: params.messageEventId,
      emoji: name,
      action: 'remove'
    });

    try {
      const conn = connection();
      const result = await createReactionAPI({
        serverId: conn.serverId ?? params.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).removeReaction({
        roomId: params.roomId,
        messageEventId: params.messageEventId,
        emoji: name
      });
      optimistic?.applyServerReaction(result.reaction);
    } catch {
      optimistic?.rollback();
      toast.error(m['room.message.reaction_failed']());
    }
  }

  async function toggleReaction(params: MessageActionParams, emoji: string, hasReacted: boolean) {
    if (hasReacted) {
      await removeReaction(params, emoji);
    } else {
      await addReaction(params, emoji);
    }
  }

  return {
    addReaction,
    removeReaction,
    toggleReaction
  };
}

/**
 * Shared message action handlers for context menu and action sheet.
 * Must be called during component initialization (uses getEditState context).
 */
export function useMessageActions() {
  const editState = getComposerContext().editState;
  const reactionActions = useReactionActions();

  function startEdit(params: MessageActionParams) {
    editState.startEdit(params.eventId, params.messageBody, {
      threadRootEventId: params.threadRootEventId,
      channelEchoEventId: params.channelEchoEventId,
      canAddChannelEcho: params.canAddChannelEcho
    });
  }

  function openDeleteConfirmation(params: MessageActionParams) {
    pushState('', {
      modal: {
        type: 'deleteMessage',
        roomId: params.roomId,
        eventId: params.deleteEventId ?? params.eventId
      }
    });
  }

  async function copyMessageLink(params: MessageActionParams) {
    await copyMessageLinkToClipboard(params.serverId, params.roomId, params.messageEventId);
  }

  return {
    ...reactionActions,
    startEdit,
    openDeleteConfirmation,
    copyMessageLink
  };
}
