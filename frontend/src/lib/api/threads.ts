import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ThreadService } from '$lib/pb/chatto/api/v1/threads_connect';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type ConnectAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
};

export function createThreadAPI(config: ConnectAPIConfig) {
	const transport = createConnectTransport({
		baseUrl: config.baseUrl,
		useBinaryFormat: true
	});
	const client = createClient(ThreadService, transport);
	const headers = () =>
		config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

	async function handleAuthError(err: unknown): Promise<never> {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
			serverRegistry.handleAuthenticationRequired(config.serverId);
		}
		throw err;
	}

	return {
		async followThread(input: { roomId: string; threadRootEventId: string }): Promise<boolean> {
			try {
				const response = await client.followThread(input, { headers: headers() });
				return response.following;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async unfollowThread(input: { roomId: string; threadRootEventId: string }): Promise<boolean> {
			try {
				const response = await client.unfollowThread(input, { headers: headers() });
				return response.following;
			} catch (err) {
				return handleAuthError(err);
			}
		}
	};
}
