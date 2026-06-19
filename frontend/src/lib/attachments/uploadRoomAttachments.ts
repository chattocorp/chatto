import { csrfFetch } from '$lib/auth/csrf';
import { UploadRoomAttachmentsResponse } from '$lib/pb/chatto/api/v1/chat_pb';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

const PROTOBUF_CONTENT_TYPE = 'application/protobuf';

export interface UploadRoomAttachmentsInput {
	roomId: string;
	files: File[];
	threadRootEventId?: string | null;
}

export async function uploadRoomAttachments({
	roomId,
	files,
	threadRootEventId
}: UploadRoomAttachmentsInput): Promise<UploadRoomAttachmentsResponse> {
	const serverId = getActiveServer();
	if (!serverId) throw new Error('No active server');

	const form = new FormData();
	for (const file of files) {
		form.append('attachments', file, file.name);
	}
	if (threadRootEventId) {
		form.set('threadRootEventId', threadRootEventId);
	}

	const response = await csrfFetch(uploadRoomAttachmentsUrl(serverId, roomId), {
		method: 'POST',
		headers: uploadRoomAttachmentsHeaders(serverId),
		body: form
	});
	if (!response.ok) {
		throw new Error(await uploadErrorMessage(response));
	}

	return UploadRoomAttachmentsResponse.fromBinary(
		new Uint8Array(await response.arrayBuffer())
	);
}

function uploadRoomAttachmentsUrl(serverId: string, roomId: string): string {
	const encodedRoomId = encodeURIComponent(roomId);
	if (serverRegistry.isOriginServer(serverId)) {
		return `/api/rooms/${encodedRoomId}/attachments`;
	}
	const server = serverRegistry.getServer(serverId);
	if (!server) throw new Error(`Server "${serverId}" not found`);
	return `${server.url.replace(/\/$/, '')}/api/rooms/${encodedRoomId}/attachments`;
}

function uploadRoomAttachmentsHeaders(serverId: string): Headers {
	const headers = new Headers({ Accept: PROTOBUF_CONTENT_TYPE });
	const token = serverRegistry.getServer(serverId)?.token;
	if (token) {
		headers.set('Authorization', `Bearer ${token}`);
	}
	return headers;
}

async function uploadErrorMessage(response: Response): Promise<string> {
	const fallback = `Attachment upload failed (${response.status})`;
	const contentType = response.headers.get('content-type') ?? '';
	if (contentType.includes('application/json')) {
		try {
			const body = (await response.json()) as { error?: unknown };
			if (typeof body.error === 'string' && body.error) return body.error;
		} catch {
			return fallback;
		}
	}
	const text = await response.text();
	return text || fallback;
}
