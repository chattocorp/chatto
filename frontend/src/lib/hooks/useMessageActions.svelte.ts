import { toast } from '$lib/ui/toast';
import { pushState } from '$app/navigation';
import { getComposerContext } from '$lib/state/room';
import { emojiToName } from '$lib/emoji';
import { tryWireAddReaction, tryWireRemoveReaction } from '$lib/wire';

export type MessageActionParams = {
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

	async function addReaction(params: MessageActionParams, emoji: string) {
		const name = emojiToName(emoji);
		if (!name) return;

		const input = {
			roomId: params.roomId,
			messageEventId: params.messageEventId,
			emoji: name
		};

		try {
			const handledByWire = await tryWireAddReaction(input);
			if (handledByWire) return;
			toast.error('Failed to add reaction');
		} catch (error) {
			toast.error('Failed to add reaction');
			console.error('Failed to add reaction:', error);
		}
	}

	async function removeReaction(params: MessageActionParams, emoji: string) {
		const name = emojiToName(emoji);
		if (!name) return;

		const input = {
			roomId: params.roomId,
			messageEventId: params.messageEventId,
			emoji: name
		};

		try {
			const handledByWire = await tryWireRemoveReaction(input);
			if (handledByWire) return;
			toast.error('Failed to remove reaction');
		} catch (error) {
			toast.error('Failed to remove reaction');
			console.error('Failed to remove reaction:', error);
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

	return { addReaction, removeReaction, toggleReaction, startEdit, openDeleteConfirmation };
}
