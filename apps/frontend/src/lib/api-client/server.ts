import { createPublicChattoClient } from "./connect.js";
import { ServerDiscoveryService } from "@chatto/api-types/chatto/discovery/v1/server_connect";
import { mapServerProfile } from "./serverProfile.js";

export type PublicAuthProvider = {
  id: string;
  type: string;
  label: string;
  loginUrl: string;
};

export type PublicServerInfo = {
  name: string;
  version: string;
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
  options: { signal?: AbortSignal } = {},
): Promise<PublicServerInfo> {
  const client = createPublicChattoClient(ServerDiscoveryService, baseUrl);
  const response = await client.getServer({}, { signal: options.signal });
  if (!response.profile?.name) {
    throw new Error("This does not appear to be a Chatto server.");
  }
  const profile = mapServerProfile(response.profile);

  return {
    name: profile.name,
    version: profile.version,
    authorizeUrl: response.login?.authorizeUrl ?? "",
    directRegistrationEnabled:
      response.login?.directRegistrationEnabled ?? false,
    welcomeMessage: profile.welcomeMessage,
    description: profile.description,
    iconUrl: profile.logoUrl,
    bannerUrl: profile.bannerUrl,
    authProviders: (response.login?.providers ?? []).map((provider) => ({
      id: provider.id,
      type: provider.type,
      label: provider.label,
      loginUrl: provider.loginUrl,
    })),
  };
}
