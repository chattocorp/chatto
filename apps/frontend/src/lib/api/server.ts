import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { ServerService } from '$lib/pb/chatto/api/v1/server_connect';

export type PublicAuthProvider = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
};

export type PublicServerInfo = {
  name: string;
  directRegistrationEnabled: boolean;
  welcomeMessage: string | null;
  description: string | null;
  iconUrl: string | null;
  bannerUrl: string | null;
  authProviders: PublicAuthProvider[];
};

export async function getPublicServerInfo(baseUrl: string): Promise<PublicServerInfo> {
  const transport = createConnectTransport({
    baseUrl: new URL('/api/connect', baseUrl).toString(),
    useBinaryFormat: true
  });
  const client = createClient(ServerService, transport);
  const response = await client.getServer({});

  return {
    name: response.name,
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
