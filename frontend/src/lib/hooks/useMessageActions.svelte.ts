import { useConnection } from '$lib/state/server/connection.svelte';
import { toast } from '$lib/ui/toast';
import { pushState } from '$app/navigation';
import { getComposerContext } from '$lib/state/room';
import { emojiToName } from '$lib/emoji';
import { copyMessageLinkToClipboard } from '$lib/messageLinks';
import {
  AddReactionFromActionsDocument,
  RemoveReactionFromActionsDocument
} from '$lib/gql/graphql';

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
};

/**
 * Shared message action handlers for context menu and action sheet.
 * Must be called during component initialization (uses getEditState context).
 */
export function useMessageActions() {
  const editState = getComposerContext().editState;
  const connection = useConnection();

  async function addReaction(params: MessageActionParams, emoji: string) {
    const name = emojiToName(emoji);
    if (!name) return;

    const result = await connection().client.mutation(AddReactionFromActionsDocument, {
      input: {
        roomId: params.roomId,
        messageEventId: params.messageEventId,
        emoji: name
      }
    });
    if (result.error) {
      toast.error('Failed to add reaction');
    }
  }

  async function removeReaction(params: MessageActionParams, emoji: string) {
    const name = emojiToName(emoji);
    if (!name) return;

    const result = await connection().client.mutation(RemoveReactionFromActionsDocument, {
      input: {
        roomId: params.roomId,
        messageEventId: params.messageEventId,
        emoji: name
      }
    });
    if (result.error) {
      toast.error('Failed to remove reaction');
    }
  }

  async function toggleReaction(params: MessageActionParams, emoji: string, hasReacted: boolean) {
    if (hasReacted) {
      await removeReaction(params, emoji);
    } else {
      await addReaction(params, emoji);
    }
  }

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
    addReaction,
    removeReaction,
    toggleReaction,
    startEdit,
    openDeleteConfirmation,
    copyMessageLink
  };
}
