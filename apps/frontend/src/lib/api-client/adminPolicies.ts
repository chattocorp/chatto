import { AdminPolicyService } from '@chatto/api-types/admin/v1/policies_connect';
import { PolicySourceScope } from '@chatto/api-types/api/v1/policies_pb';
import { authHeaders, createChattoClient, handleAuthError } from './connect.js';

export type AdminPolicyAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
	onAuthenticationRequired?: (serverId: string) => void;
};

export type PolicyScope =
	| { kind: 'server' }
	| { kind: 'room-group'; id: string }
	| { kind: 'room'; id: string };

export type PolicySource = {
	kind: 'product-default' | 'server' | 'room-group' | 'room' | 'unknown';
	id: string | null;
};

export type PolicyConfiguration = {
	authorEditWindowSeconds: number | null;
	effectiveAuthorEditWindowSeconds: number;
	authorEditWindowSource: PolicySource;
};

function apiScope(scope: PolicyScope) {
	switch (scope.kind) {
		case 'server':
			return { scope: { case: 'server' as const, value: true } };
		case 'room-group':
			return { scope: { case: 'roomGroupId' as const, value: scope.id } };
		case 'room':
			return { scope: { case: 'roomId' as const, value: scope.id } };
	}
}

function mapSource(source: { scope?: PolicySourceScope; scopeId?: string } | undefined): PolicySource {
	let kind: PolicySource['kind'] = 'unknown';
	switch (source?.scope) {
		case PolicySourceScope.PRODUCT_DEFAULT:
			kind = 'product-default';
			break;
		case PolicySourceScope.SERVER:
			kind = 'server';
			break;
		case PolicySourceScope.ROOM_GROUP:
			kind = 'room-group';
			break;
		case PolicySourceScope.ROOM:
			kind = 'room';
			break;
	}
	return { kind, id: source?.scopeId || null };
}

function mapConfiguration(
	configuration:
		| {
				overrides?: { authorEditWindowSeconds?: number };
				effective?: { authorEditWindowSeconds?: number };
				sources?: { authorEditWindow?: { scope?: PolicySourceScope; scopeId?: string } };
		  }
		| undefined
): PolicyConfiguration {
	return {
		authorEditWindowSeconds: configuration?.overrides?.authorEditWindowSeconds ?? null,
		effectiveAuthorEditWindowSeconds:
			configuration?.effective?.authorEditWindowSeconds ?? 3 * 60 * 60,
		authorEditWindowSource: mapSource(configuration?.sources?.authorEditWindow)
	};
}

export function createAdminPolicyAPI(config: AdminPolicyAPIConfig) {
	const policies = createChattoClient(AdminPolicyService, config);
	const headers = () => authHeaders(config);
	return {
		async getPolicyConfiguration(
			scope: PolicyScope,
			options: { signal?: AbortSignal } = {}
		): Promise<PolicyConfiguration> {
			try {
				const response = await policies.getPolicyConfiguration(
					{ scope: apiScope(scope) },
					{ headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
				);
				return mapConfiguration(response.policyConfiguration);
			} catch (error) {
				return handleAuthError(config, error);
			}
		},

		async updatePolicyConfiguration(
			scope: PolicyScope,
			authorEditWindowSeconds: number | null
		): Promise<PolicyConfiguration> {
			try {
				const response = await policies.updatePolicyConfiguration(
					{
						scope: apiScope(scope),
						overrides: {
							authorEditWindowSeconds: authorEditWindowSeconds ?? undefined
						},
						updateMask: { paths: ['author_edit_window_seconds'] }
					},
					{ headers: headers() }
				);
				return mapConfiguration(response.policyConfiguration);
			} catch (error) {
				return handleAuthError(config, error);
			}
		}
	};
}
