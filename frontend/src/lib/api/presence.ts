import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { PresenceService } from '$lib/pb/chatto/api/v1/presence_connect';
import { PresenceStatus } from '$lib/pb/chatto/api/v1/presence_pb';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type PresenceAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
};

export { PresenceStatus as APIPresenceStatus };

export function createPresenceAPI(config: PresenceAPIConfig) {
	const transport = createConnectTransport({
		baseUrl: config.baseUrl,
		useBinaryFormat: true
	});
	const client = createClient(PresenceService, transport);
	const headers = () =>
		config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

	async function handleAuthError(err: unknown): Promise<never> {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
			serverRegistry.handleAuthenticationRequired(config.serverId);
		}
		throw err;
	}

	return {
		async reportPresence(status: PresenceStatus, userSelected = false): Promise<PresenceStatus> {
			try {
				const response = await client.reportPresence({ status, userSelected }, { headers: headers() });
				return response.status;
			} catch (err) {
				return handleAuthError(err);
			}
		}
	};
}
