import { AdminOAuthClientService } from '@chatto/api-types/admin/v1/oauth_clients_connect';
import {
  OAuthClientPolicy,
  OAuthClientSource,
  type OAuthClient as APIOAuthClient
} from '@chatto/api-types/admin/v1/oauth_clients_pb';
import { authHeaders, createChattoClient } from './connect.js';

export type OAuthClientPolicyName = 'default' | 'trusted' | 'blocked';

export type OAuthClient = {
  clientId: string;
  clientName: string;
  clientOrigin: string;
  source: 'cimd' | 'built-in';
  policy: OAuthClientPolicyName;
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
    async updatePolicy(clientId: string, policy: OAuthClientPolicyName) {
      const response = await client.updateOAuthClientPolicy(
        { clientId, policy: apiPolicy(policy) },
        { headers: headers() }
      );
      if (!response.oauthClient) throw new Error('OAuth-client response was incomplete.');
      return mapOAuthClient(response.oauthClient);
    }
  };
}

function mapOAuthClient(client: APIOAuthClient): OAuthClient {
  return {
    clientId: client.clientId,
    clientName: client.clientName,
    clientOrigin: client.clientOrigin,
    source:
      client.source === OAuthClientSource.OAUTH_CLIENT_SOURCE_BUILT_IN ? 'built-in' : 'cimd',
    policy: policyName(client.policy),
    firstAuthorizationAt: client.firstAuthorizationAt?.toDate().toISOString() ?? '',
    lastAuthorizationAt: client.lastAuthorizationAt?.toDate().toISOString() ?? '',
    redirectOrigins: [...client.redirectOrigins],
    authorizedUserCount: client.authorizedUserCount
  };
}

function policyName(policy: OAuthClientPolicy): OAuthClientPolicyName {
  if (policy === OAuthClientPolicy.OAUTH_CLIENT_POLICY_TRUSTED) return 'trusted';
  if (policy === OAuthClientPolicy.OAUTH_CLIENT_POLICY_BLOCKED) return 'blocked';
  return 'default';
}

function apiPolicy(policy: OAuthClientPolicyName): OAuthClientPolicy {
  if (policy === 'trusted') return OAuthClientPolicy.OAUTH_CLIENT_POLICY_TRUSTED;
  if (policy === 'blocked') return OAuthClientPolicy.OAUTH_CLIENT_POLICY_BLOCKED;
  return OAuthClientPolicy.OAUTH_CLIENT_POLICY_DEFAULT;
}
