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
  /** Absent on older servers, where setup is unavailable. */
  setupRequired?: boolean;
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
  // Discovery must settle so startup recovery can retry a stalled endpoint.
  const response = await client.getServer({}, { signal: options.signal, timeoutMs: 10_000 });
  if (!response.profile?.name) {
    throw new InvalidPublicServerError('The response has no public Chatto server profile.');
  }
  const profile = mapServerProfile(response.profile);

  return {
    setupRequired: response.setupRequired,
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

/** Public profile fields of one server in a Neighborhood. */
export type NeighborhoodServerProfile = Pick<
  PublicServerInfo,
  'name' | 'version' | 'description' | 'iconUrl' | 'bannerUrl'
>;

/** One server in the Neighborhood that a Chatto server discovered. */
export type NeighborhoodServer = {
  origin: string;
  /** Logo and banner URLs identify copies on the called server. */
  profile: NeighborhoodServerProfile;
  /** Whether the called server advertises this server as a Neighbor. */
  directNeighbor: boolean;
  /** Other servers in the same response that mutually recommend this server. */
  recommendedByOrigins: string[];
};

/**
 * Read the Neighborhood that one server discovered and cached. The request goes
 * only to `baseUrl`; the called server does not contact other servers for it.
 */
export async function listNeighborhoodServers(
  baseUrl: string,
  options: { signal?: AbortSignal } = {}
): Promise<NeighborhoodServer[]> {
  const client = createPublicChattoClient(ServerDiscoveryService, baseUrl);
  const response = await client.listNeighborhoodServers(
    {},
    { signal: options.signal, timeoutMs: 10_000 }
  );
  return response.servers.flatMap((server) => {
    if (!server.origin || !server.profile?.name) return [];
    const profile = mapServerProfile(server.profile);
    return [
      {
        origin: server.origin,
        profile: {
          name: profile.name,
          version: profile.version,
          description: profile.description,
          iconUrl: profile.logoUrl,
          bannerUrl: profile.bannerUrl
        },
        directNeighbor: server.directNeighbor,
        recommendedByOrigins: [...server.recommendedByOrigins]
      }
    ];
  });
}
