import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig
} from './connect.js';
import { ExternalIdentityAuthService } from '@chatto/api-types/chatto/auth/v1/external_identity_auth_connect';
import {
  ExternalIdentityFlowKind,
  type PendingExternalIdentity as APIPendingExternalIdentity
} from '@chatto/api-types/chatto/auth/v1/external_identity_auth_pb';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import {
  type ExternalIdentityProvider as APIExternalIdentityProvider,
  type LinkedExternalIdentity as APILinkedExternalIdentity
} from '@chatto/api-types/api/v1/external_identities_pb';

export type ExternalIdentityFlowAPIConfig = {
  baseUrl?: string;
};

export type ExternalIdentityAPIConfig = ConnectAPIConfig;

export type PendingExternalIdentityInfo = {
  kind: ExternalIdentityFlowKind;
  providerId: string;
  providerType: string;
  providerLabel: string;
  verifiedEmail: string | null;
  loginHint: string;
  displayNameHint: string;
  boundUserId: string | null;
  redirectPath: string | null;
};

export type LinkedExternalIdentityInfo = {
  providerId: string;
  providerType: string;
  providerLabel: string;
  subjectHash: string;
};

export type ExternalIdentityProviderInfo = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
  linkUrl: string;
  linked: boolean;
  linkedIdentitySubjectHash: string | null;
};

export type ExternalIdentityList = {
  providers: ExternalIdentityProviderInfo[];
  linkedIdentities: LinkedExternalIdentityInfo[];
};

export type CreatedExternalIdentityAccount = {
  userId: string;
  login: string;
  token: string;
};

export function createExternalIdentityFlowAPI(config: ExternalIdentityFlowAPIConfig = {}) {
  const client = createChattoClient(ExternalIdentityAuthService, {
    baseUrl: config.baseUrl ?? '/api/connect'
  });

  return {
    async getPending(token: string): Promise<PendingExternalIdentityInfo | null> {
      const response = await client.getPendingExternalIdentity({ token });
      return pendingIdentity(response.pending);
    },

    async createAccount(input: {
      token: string;
      login: string;
    }): Promise<CreatedExternalIdentityAccount> {
      const response = await client.createExternalIdentityAccount(input);
      return {
        userId: response.userId,
        login: response.login,
        token: response.token
      };
    },

    async cancel(token: string): Promise<void> {
      await client.cancelExternalIdentityFlow({ token });
    },

    async confirmLink(token: string): Promise<LinkedExternalIdentityInfo | null> {
      const response = await client.confirmExternalIdentityLink({ token });
      return linkedIdentity(response.linkedIdentity);
    }
  };
}

export function createExternalIdentityAPI(config: ExternalIdentityAPIConfig) {
  const client = createChattoClient(MyAccountService, config);
  const headers = () => authHeaders(config);

  return {
    async list(): Promise<ExternalIdentityList> {
      try {
        const response = await client.listExternalIdentities({}, { headers: headers() });
        return {
          providers: response.providers.map((provider) =>
            externalIdentityProvider(provider, config.baseUrl)
          ),
          linkedIdentities: response.linkedIdentities.map(linkedIdentity).filter(isLinkedIdentity)
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async startLink(input: {
      providerId: string;
      redirectPath: string;
      currentPassword?: string;
    }): Promise<string> {
      try {
        const response = await client.startExternalIdentityLink(input, {
          headers: headers()
        });
        return response.startUrl;
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async disconnect(subjectHash: string, currentPassword?: string): Promise<void> {
      try {
        await client.disconnectExternalIdentity(
          { subjectHash, currentPassword },
          { headers: headers() }
        );
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
  };
}

export { ExternalIdentityFlowKind };

function pendingIdentity(pending?: APIPendingExternalIdentity): PendingExternalIdentityInfo | null {
  if (!pending) return null;
  return {
    kind: pending.kind,
    providerId: pending.providerId,
    providerType: pending.providerType,
    providerLabel: pending.providerLabel,
    verifiedEmail: pending.verifiedEmail || null,
    loginHint: pending.loginHint,
    displayNameHint: pending.displayNameHint,
    boundUserId: pending.boundUserId || null,
    redirectPath: pending.redirectPath || null
  };
}

function externalIdentityProvider(
  provider: APIExternalIdentityProvider,
  baseUrl: string
): ExternalIdentityProviderInfo {
  const metadata = provider.provider;
  if (!metadata) {
    throw new Error('external identity provider response did not include provider metadata');
  }
  return {
    id: metadata.id,
    type: metadata.type,
    label: metadata.label,
    loginUrl: resolveServerUrl(metadata.loginUrl, baseUrl),
    linkUrl: resolveServerUrl(provider.linkUrl, baseUrl),
    linked: provider.linked,
    linkedIdentitySubjectHash: provider.linkedIdentitySubjectHash || null
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

function linkedIdentity(identity?: APILinkedExternalIdentity): LinkedExternalIdentityInfo | null {
  if (!identity) return null;
  return {
    providerId: identity.providerId,
    providerType: identity.providerType,
    providerLabel: identity.providerLabel,
    subjectHash: identity.subjectHash
  };
}

function isLinkedIdentity(
  identity: LinkedExternalIdentityInfo | null
): identity is LinkedExternalIdentityInfo {
  return identity !== null;
}
