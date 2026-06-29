import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ServerDiscoveryService } from '@chatto/api-types/api/v1/server_connect';

export type PublicAuthProvider = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
};

export type PublicServerInfo = {
  name: string;
  version: string;
  authMethods: string[];
  authorizeUrl: string;
  directRegistrationEnabled: boolean;
  welcomeMessage: string | null;
  description: string | null;
  iconUrl: string | null;
  bannerUrl: string | null;
  authProviders: PublicAuthProvider[];
};

export async function getPublicServerInfo(
  baseUrl: string,
  options: { signal?: AbortSignal } = {}
): Promise<PublicServerInfo> {
  const transport = createConnectTransport({
    baseUrl: new URL('/api/connect', baseUrl).toString()
  });
  const client = createClient(ServerDiscoveryService, transport);
  const response = await client.getServer({}, { signal: options.signal });

  return {
    name: response.name,
    version: response.version,
    authMethods: [...response.authMethods],
    authorizeUrl: response.authorizeUrl,
    directRegistrationEnabled: response.registrationOpen,
    welcomeMessage: response.welcomeMessage || null,
    description: response.description || null,
    iconUrl: response.logoUrl || null,
    bannerUrl: response.bannerUrl || null,
    authProviders: response.authProviders.map((provider) => ({
      id: provider.id,
      type: provider.type,
      label: provider.label,
      loginUrl: provider.loginUrl
    }))
  };
}
