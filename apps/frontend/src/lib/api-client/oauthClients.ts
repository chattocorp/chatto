import { AdminOAuthClientService } from '@chatto/api-types/admin/v1/oauth_clients_connect';
import {
  OAuthClientPolicy,
  OAuthClientSource,
  type OAuthClient as APIOAuthClient
} from '@chatto/api-types/admin/v1/oauth_clients_pb';
import { authHeaders, createChattoClient } from './connect.js';

export type EditableOAuthClientPolicyName = 'default' | 'trusted' | 'blocked';
export type OAuthClientPolicyName = EditableOAuthClientPolicyName | 'unknown';
export type OAuthClientSourceName = 'cimd' | 'built-in' | 'unknown';

export type OAuthClient = {
  clientId: string;
  clientName: string;
  clientOrigin: string;
  source: OAuthClientSourceName;
  sourceCode: number;
  policy: OAuthClientPolicyName;
  policyCode: number;
  firstAuthorizationAt: string;
  lastAuthorizationAt: string;
  redirectOrigins: string[];
  authorizedUserCount: number;
};

export type OAuthClientAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export function createOAuthClientAPI(config: OAuthClientAPIConfig) {
  const client = createChattoClient(AdminOAuthClientService, config);
  const headers = () => authHeaders(config);
  return {
    async list(offset = 0, limit = 100, options: { signal?: AbortSignal } = {}) {
      const response = await client.listOAuthClients(
        { page: { offset, limit } },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        oauthClients: response.oauthClients.map(mapOAuthClient),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async updatePolicy(clientId: string, policy: EditableOAuthClientPolicyName) {
      const response = await client.updateOAuthClientPolicy(
        { clientId, policy: apiPolicy(policy) },
        { headers: headers() }
      );
      if (!response.oauthClient) throw new Error('OAuth-client response was incomplete.');
      return mapOAuthClient(response.oauthClient);
    }
  };
}

export function mapOAuthClient(client: APIOAuthClient): OAuthClient {
  return {
    clientId: client.clientId,
    clientName: client.clientName,
    clientOrigin: client.clientOrigin,
    source: sourceName(client.source),
    sourceCode: client.source,
    policy: policyName(client.policy),
    policyCode: client.policy,
    firstAuthorizationAt: client.firstAuthorizationAt?.toDate().toISOString() ?? '',
    lastAuthorizationAt: client.lastAuthorizationAt?.toDate().toISOString() ?? '',
    redirectOrigins: [...client.redirectOrigins],
    authorizedUserCount: client.authorizedUserCount
  };
}

function sourceName(source: OAuthClientSource): OAuthClientSourceName {
  if (source === OAuthClientSource.OAUTH_CLIENT_SOURCE_CIMD) return 'cimd';
  if (source === OAuthClientSource.OAUTH_CLIENT_SOURCE_BUILT_IN) return 'built-in';
  return 'unknown';
}

function policyName(policy: OAuthClientPolicy): OAuthClientPolicyName {
  if (policy === OAuthClientPolicy.OAUTH_CLIENT_POLICY_DEFAULT) return 'default';
  if (policy === OAuthClientPolicy.OAUTH_CLIENT_POLICY_TRUSTED) return 'trusted';
  if (policy === OAuthClientPolicy.OAUTH_CLIENT_POLICY_BLOCKED) return 'blocked';
  return 'unknown';
}

function apiPolicy(policy: EditableOAuthClientPolicyName): OAuthClientPolicy {
  if (policy === 'default') return OAuthClientPolicy.OAUTH_CLIENT_POLICY_DEFAULT;
  if (policy === 'trusted') return OAuthClientPolicy.OAUTH_CLIENT_POLICY_TRUSTED;
  if (policy === 'blocked') return OAuthClientPolicy.OAUTH_CLIENT_POLICY_BLOCKED;
  throw new Error('Unsupported OAuth client policy.');
}
