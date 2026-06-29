import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import type { LinkPreviewInput, RoomEventView } from '$lib/render/types';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { MessageAttachmentUpload } from '@chatto/api-types/api/v1/messages_pb';
import { LinkPreview } from '@chatto/api-types/api/v1/link_previews_pb';
import { roomTimelineEventToRawEvent } from '$lib/api/roomTimeline';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type MessageAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
};

export type PostMessageInput = {
	roomId: string;
	body: string;
	attachmentAssetIds?: string[];
	attachments?: File[] | null;
	threadRootEventId?: string | null;
	inReplyTo?: string | null;
	alsoSendToChannel?: boolean;
	mentionConfirmationToken?: string | null;
	linkPreview?: LinkPreviewInput | null;
};

export type UpdateMessageInput = {
	roomId: string;
	eventId: string;
	body: string;
	alsoSendToChannel?: boolean;
};

export type PostMessageResult =
	| {
			kind: 'event';
			event: RoomEventView | null;
	  }
	| {
			kind: 'mentionConfirmation';
			recipientCount: number;
			token: string;
	  };

export function createMessageAPI(config: MessageAPIConfig) {
	const transport = createConnectTransport({
		baseUrl: config.baseUrl,
		useBinaryFormat: true
	});
	const client = createClient(MessageService, transport);
	const headers = () =>
		config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

	async function handleAuthError(err: unknown): Promise<never> {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
			serverRegistry.handleAuthenticationRequired(config.serverId);
		}
		throw err;
	}

	return {
		async postMessage(input: PostMessageInput): Promise<PostMessageResult> {
			try {
				const response = await client.postMessage(
					{
						roomId: input.roomId,
						body: input.body,
						attachmentAssetIds: input.attachmentAssetIds ?? [],
						attachments: await messageAttachmentUploads(input.attachments),
						threadRootEventId: input.threadRootEventId ?? '',
						inReplyTo: input.inReplyTo ?? '',
						alsoSendToChannel: input.alsoSendToChannel ?? false,
						mentionConfirmationToken: input.mentionConfirmationToken ?? '',
						linkPreview: messageLinkPreviewInput(input.linkPreview)
					},
					{ headers: headers() }
				);

				if (response.result.case === 'mentionConfirmation') {
					return {
						kind: 'mentionConfirmation',
						recipientCount: response.result.value.recipientCount,
						token: response.result.value.token
					};
				}

				if (response.result.case === 'event') {
					return {
						kind: 'event',
						event: roomTimelineEventToRawEvent(
							response.result.value,
							response.includes?.users ?? {}
						) as RoomEventView | null
					};
				}

				return { kind: 'event', event: null };
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async updateMessage(input: UpdateMessageInput): Promise<boolean> {
			try {
				const response = await client.updateMessage(
					{
						roomId: input.roomId,
						eventId: input.eventId,
						body: input.body,
						alsoSendToChannel: input.alsoSendToChannel
					},
					{ headers: headers() }
				);
				return response.updated;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async deleteMessage(roomId: string, eventId: string): Promise<boolean> {
			try {
				const response = await client.deleteMessage({ roomId, eventId }, { headers: headers() });
				return response.deleted;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async deleteAttachment(roomId: string, eventId: string, attachmentId: string): Promise<boolean> {
			try {
				const response = await client.deleteAttachment(
					{ roomId, eventId, attachmentId },
					{ headers: headers() }
				);
				return response.deleted;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async deleteLinkPreview(roomId: string, eventId: string, url: string): Promise<boolean> {
			try {
				const response = await client.deleteLinkPreview({ roomId, eventId, url }, { headers: headers() });
				return response.deleted;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async sendTypingIndicator(roomId: string, threadRootEventId?: string | null): Promise<boolean> {
			try {
				const response = await client.sendTypingIndicator(
					{
						roomId,
						threadRootEventId: threadRootEventId ?? ''
					},
					{ headers: headers() }
				);
				return response.sent;
			} catch (err) {
				return handleAuthError(err);
			}
		}
	};
}

async function messageAttachmentUploads(files: File[] | null | undefined) {
	if (!files?.length) return [];
	return Promise.all(
		files.map(async (file) => {
			const buffer = await file.arrayBuffer();
			return new MessageAttachmentUpload({
				content: new Uint8Array(buffer),
				filename: file.name,
				contentType: file.type || 'application/octet-stream'
			});
		})
	);
}

function messageLinkPreviewInput(input: LinkPreviewInput | null | undefined) {
	if (!input) return undefined;
	return new LinkPreview({
		url: input.url,
		title: input.title ?? undefined,
		description: input.description ?? undefined,
		siteName: input.siteName ?? undefined,
		imageAssetId: input.imageAssetId ?? undefined,
		embedType: input.embedType ?? undefined,
		embedId: input.embedId ?? undefined
	});
}
