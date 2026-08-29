import { createPublicChattoClient } from './connect.js';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import { AccountCreationPolicy } from '@chatto/api-types/api/v1/server_pb';
import { mapServerProfile } from './serverProfile.js';

export type PublicAuthProvider = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
  issuerUrl: string | null;
  autoProvision: boolean | null;
};

export type PublicServerInfo = {
  name: string;
  version: string;
  authorizeUrl: string;
  directRegistrationEnabled: boolean;
  directLoginEnabled: boolean;
  accountCreationPolicy: 'open' | 'invite_only';
  welcomeMessage: string | null;
  description: string | null;
  iconUrl: string | null;
  bannerUrl: string | null;
  authProviders: PublicAuthProvider[];
};

/** The discovery response did not contain a valid public Chatto server profile. */
export class InvalidPublicServerError extends Error {}

export async function getPublicServerInfo(
  baseUrl: string,
  options: { signal?: AbortSignal } = {}
): Promise<PublicServerInfo> {
  const client = createPublicChattoClient(ServerDiscoveryService, baseUrl);
  const response = await client.getServer({}, { signal: options.signal });
  if (!response.profile?.name) {
    throw new InvalidPublicServerError('The response has no public Chatto server profile.');
  }
  const profile = mapServerProfile(response.profile);

  return {
    name: profile.name,
    version: profile.version,
    authorizeUrl: response.login?.authorizeUrl ?? '',
    directRegistrationEnabled: response.login?.directRegistrationEnabled ?? false,
    directLoginEnabled: response.login?.directLoginEnabled ?? true,
    accountCreationPolicy:
      response.login?.accountCreationPolicy === AccountCreationPolicy.INVITE_ONLY
        ? 'invite_only'
        : 'open',
    welcomeMessage: profile.welcomeMessage,
    description: profile.description,
    iconUrl: profile.logoUrl,
    bannerUrl: profile.bannerUrl,
    authProviders: (response.login?.providers ?? []).map((provider) => ({
      id: provider.id,
      type: provider.type,
      label: provider.label,
      loginUrl: provider.loginUrl,
      issuerUrl: provider.issuerUrl ?? null,
      autoProvision: provider.autoProvision ?? null
    }))
  };
}

/** Read one server's public Neighbor origins without requiring a session. */
export async function getPublicNeighborOrigins(
  baseUrl: string,
  options: { signal?: AbortSignal } = {}
): Promise<string[]> {
  const client = createPublicChattoClient(ServerDiscoveryService, baseUrl);
  const response = await client.listNeighbors({}, { signal: options.signal });
  return response.origins;
}
