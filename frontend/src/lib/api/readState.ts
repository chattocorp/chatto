import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ReadStateService } from '$lib/pb/chatto/api/v1/read_state_connect';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type ConnectAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
};

export type MarkRoomAsReadResult = {
	lastReadAt: string | null;
	previousLastReadAt: string | null;
};

export type MarkThreadAsReadResult = {
	previousReadAt: string | null;
};

export function createReadStateAPI(config: ConnectAPIConfig) {
	const transport = createConnectTransport({
		baseUrl: config.baseUrl,
		useBinaryFormat: true
	});
	const client = createClient(ReadStateService, transport);
	const headers = () =>
		config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

	async function handleAuthError(err: unknown): Promise<never> {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
			serverRegistry.handleAuthenticationRequired(config.serverId);
		}
		throw err;
	}

	return {
		async markRoomAsRead(input: {
			roomId: string;
			upToEventId?: string;
		}): Promise<MarkRoomAsReadResult> {
			try {
				const response = await client.markRoomAsRead(
					{
						roomId: input.roomId,
						upToEventId: input.upToEventId ?? ''
					},
					{ headers: headers() }
				);
				return {
					lastReadAt: response.lastReadAt?.toDate().toISOString() ?? null,
					previousLastReadAt: response.previousLastReadAt?.toDate().toISOString() ?? null
				};
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async markThreadAsRead(input: {
			roomId: string;
			threadRootEventId: string;
			upToEventId?: string;
		}): Promise<MarkThreadAsReadResult> {
			try {
				const response = await client.markThreadAsRead(
					{
						roomId: input.roomId,
						threadRootEventId: input.threadRootEventId,
						upToEventId: input.upToEventId ?? ''
					},
					{ headers: headers() }
				);
				return {
					previousReadAt: response.previousReadAt?.toDate().toISOString() ?? null
				};
			} catch (err) {
				return handleAuthError(err);
			}
		}
	};
}
