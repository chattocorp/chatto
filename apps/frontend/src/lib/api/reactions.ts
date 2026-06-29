import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ReactionService } from '@chatto/api-types/api/v1/reactions_connect';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type ConnectAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
};

export type ReactionInput = {
	roomId: string;
	messageEventId: string;
	emoji: string;
};

export function createReactionAPI(config: ConnectAPIConfig) {
	const transport = createConnectTransport({
		baseUrl: config.baseUrl,
		useBinaryFormat: true
	});
	const client = createClient(ReactionService, transport);
	const headers = () =>
		config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

	async function handleAuthError(err: unknown): Promise<never> {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
			serverRegistry.handleAuthenticationRequired(config.serverId);
		}
		throw err;
	}

	return {
		async addReaction(input: ReactionInput): Promise<boolean> {
			try {
				const response = await client.addReaction(input, { headers: headers() });
				return response.added;
			} catch (err) {
				return handleAuthError(err);
			}
		},

		async removeReaction(input: ReactionInput): Promise<boolean> {
			try {
				const response = await client.removeReaction(input, { headers: headers() });
				return response.removed;
			} catch (err) {
				return handleAuthError(err);
			}
		}
	};
}
