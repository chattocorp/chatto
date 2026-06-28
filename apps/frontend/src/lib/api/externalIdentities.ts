import { Code, ConnectError, createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import {
  ExternalIdentityFlowService,
  ExternalIdentityService
} from '$lib/pb/chatto/api/v1/external_identities_connect';
import {
  ExternalIdentityFlowKind,
  type ExternalIdentityProvider,
  type LinkedExternalIdentity,
  type PendingExternalIdentity
} from '$lib/pb/chatto/api/v1/external_identities_pb';
import { serverRegistry } from '$lib/state/server/registry.svelte';

export type ExternalIdentityAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
};

export type PendingSSOIdentity = {
  kind: ExternalIdentityFlowKind;
  providerId: string;
  providerType: string;
  providerLabel: string;
  verifiedEmail: string | null;
  loginHint: string;
  displayNameHint: string;
  boundUserId: string | null;
};

export type LinkedSSOIdentity = {
  providerId: string;
  providerType: string;
  providerLabel: string;
  subjectHash: string;
};

export type SSOProvider = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
  linkUrl: string;
};

export function createExternalIdentityFlowAPI(baseUrl = '/api/connect') {
  const transport = createConnectTransport({
    baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(ExternalIdentityFlowService, transport);

  return {
    async getPending(token: string): Promise<PendingSSOIdentity | null> {
      const response = await client.getPendingExternalIdentity({ token });
      return pendingIdentity(response.pending);
    },

    async createAccount(input: {
      token: string;
      login: string;
    }): Promise<{ userId: string; login: string; token: string }> {
      const response = await client.createExternalIdentityAccount(input);
      return {
        userId: response.userId,
        login: response.login,
        token: response.token
      };
    },

    async cancel(token: string): Promise<void> {
      await client.cancelExternalIdentityFlow({ token });
    }
  };
}

export function createExternalIdentityAPI(config: ExternalIdentityAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(ExternalIdentityService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (err instanceof ConnectError && err.code === Code.Unauthenticated && config.serverId) {
      serverRegistry.handleAuthenticationRequired(config.serverId);
    }
    throw err;
  }

  return {
    async list(): Promise<{ providers: SSOProvider[]; linkedIdentities: LinkedSSOIdentity[] }> {
      try {
        const response = await client.listExternalIdentities({}, { headers: headers() });
        return {
          providers: response.providers.map((provider) => ssoProvider(provider, config.baseUrl)),
          linkedIdentities: response.linkedIdentities.map(linkedIdentity).filter(isLinkedIdentity)
        };
      } catch (err) {
        return handleAuthError(err);
      }
    },

    async link(token: string): Promise<LinkedSSOIdentity | null> {
      try {
        const response = await client.linkExternalIdentity({ token }, { headers: headers() });
        return linkedIdentity(response.linkedIdentity);
      } catch (err) {
        return handleAuthError(err);
      }
    }
  };
}

export { ExternalIdentityFlowKind };

function pendingIdentity(pending?: PendingExternalIdentity): PendingSSOIdentity | null {
  if (!pending) return null;
  return {
    kind: pending.kind,
    providerId: pending.providerId,
    providerType: pending.providerType,
    providerLabel: pending.providerLabel,
    verifiedEmail: pending.verifiedEmail || null,
    loginHint: pending.loginHint,
    displayNameHint: pending.displayNameHint,
    boundUserId: pending.boundUserId || null
  };
}

function ssoProvider(provider: ExternalIdentityProvider, baseUrl: string): SSOProvider {
  return {
    id: provider.id,
    type: provider.type,
    label: provider.label,
    loginUrl: resolveServerUrl(provider.loginUrl, baseUrl),
    linkUrl: resolveServerUrl(provider.linkUrl, baseUrl)
  };
}

function resolveServerUrl(value: string, baseUrl: string): string {
  if (!value) return value;
  try {
    const base = new URL(baseUrl, globalThis.location?.origin ?? 'http://localhost');
    return new URL(value, base.origin).toString();
  } catch {
    return value;
  }
}

function linkedIdentity(identity?: LinkedExternalIdentity): LinkedSSOIdentity | null {
  if (!identity) return null;
  return {
    providerId: identity.providerId,
    providerType: identity.providerType,
    providerLabel: identity.providerLabel,
    subjectHash: identity.subjectHash
  };
}

function isLinkedIdentity(identity: LinkedSSOIdentity | null): identity is LinkedSSOIdentity {
  return identity !== null;
}
