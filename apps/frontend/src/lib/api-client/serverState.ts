import { authHeaders, createChattoClient } from './connect.js';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import { mapServerProfile, type ServerProfile } from './serverProfile.js';

export type ServerStateAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type AuthenticatedServerState = {
  name: string;
  version: string;
  logoUrl: string | null;
  bannerUrl: string | null;
  welcomeMessage: string | null;
  description: string | null;
  motd: string | null;
  pushNotificationsEnabled: boolean;
  vapidPublicKey: string | null;
  livekitUrl: string | null;
  videoProcessingEnabled: boolean;
  maxUploadSize: number;
  maxVideoUploadSize: number;
  messageEditWindowSeconds: number;
};

export type EditableServerConfig = {
  name: string;
  description: string;
  motd: string;
  welcomeMessage: string;
};

export type EditableServerProfile = ServerProfile;

export type ServerSecurityConfig = {
  blockedUsernames: string;
};

function serverClients(config: ServerStateAPIConfig) {
  const discovery = createChattoClient(ServerDiscoveryService, config);
  const server = createChattoClient(ServerService, config);
  const adminServer = createChattoClient(AdminServerService, config);
  const headers = authHeaders(config);
  return { discovery, server, adminServer, headers };
}

function mapEditableServerConfig(
  config:
    | {
        serverName?: string;
        description?: string;
        motd?: string;
        welcomeMessage?: string;
      }
    | null
    | undefined
): EditableServerConfig {
  return {
    name: config?.serverName ?? '',
    description: config?.description ?? '',
    motd: config?.motd ?? '',
    welcomeMessage: config?.welcomeMessage ?? ''
  };
}

function blockedUsernamesText(entries: readonly string[] | undefined): string {
  return (entries ?? []).join('\n');
}

function blockedUsernameEntries(text: string): string[] {
  return text
    .split('\n')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export async function getAuthenticatedServerState(
  config: ServerStateAPIConfig
): Promise<AuthenticatedServerState> {
  const { discovery, server, headers } = serverClients(config);
  const [discoveryResponse, motdResponse, runtimeResponse] = await Promise.all([
    discovery.getServer({}),
    server.getMotd({}, { headers }),
    server.getRuntimeConfig({}, { headers })
  ]);
  const profile = mapServerProfile(discoveryResponse.profile);
  const runtime = runtimeResponse.runtime;

  return {
    name: profile.name,
    version: profile.version,
    logoUrl: profile.logoUrl,
    bannerUrl: profile.bannerUrl,
    welcomeMessage: profile.welcomeMessage,
    description: profile.description,
    motd: motdResponse.motd ?? null,
    pushNotificationsEnabled: runtime?.pushNotificationsEnabled ?? false,
    vapidPublicKey: runtime?.vapidPublicKey ?? null,
    livekitUrl: runtime?.livekitUrl ?? null,
    videoProcessingEnabled: runtime?.videoProcessingEnabled ?? false,
    maxUploadSize: Number(runtime?.maxUploadSize ?? 0),
    maxVideoUploadSize: Number(runtime?.maxVideoUploadSize ?? 0),
    messageEditWindowSeconds: runtime?.messageEditWindowSeconds ?? 0
  };
}

export async function getServerConfig(config: ServerStateAPIConfig): Promise<EditableServerConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.getServerConfig({}, { headers });
  return mapEditableServerConfig(response.config);
}

export async function updateServerConfig(
  config: ServerStateAPIConfig,
  input: EditableServerConfig
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.updateServerConfig(
    {
      serverName: input.name,
      description: input.description,
      motd: input.motd,
      welcomeMessage: input.welcomeMessage
    },
    { headers }
  );

  return mapServerProfile({ publicProfile: response.publicProfile, motd: response.config?.motd });
}

export async function uploadServerLogo(
  config: ServerStateAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerLogo(
    {
      image: {
        image: new Uint8Array(await file.arrayBuffer()),
        filename: file.name,
        contentType: file.type
      }
    },
    { headers }
  );
  return mapServerProfile(response.publicProfile);
}

export async function deleteServerLogo(
  config: ServerStateAPIConfig
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerLogo({}, { headers });
  return mapServerProfile(response.publicProfile);
}

export async function uploadServerBanner(
  config: ServerStateAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerBanner(
    {
      image: {
        image: new Uint8Array(await file.arrayBuffer()),
        filename: file.name,
        contentType: file.type
      }
    },
    { headers }
  );
  return mapServerProfile(response.publicProfile);
}

export async function deleteServerBanner(
  config: ServerStateAPIConfig
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerBanner({}, { headers });
  return mapServerProfile(response.publicProfile);
}

export async function getServerSecurityConfig(
  config: ServerStateAPIConfig
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.getServerSecurityConfig({}, { headers });
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames)
  };
}

export async function updateBlockedUsernames(
  config: ServerStateAPIConfig,
  blockedUsernames: string
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.updateBlockedUsernames(
    { blockedUsernames: blockedUsernameEntries(blockedUsernames) },
    { headers }
  );
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames)
  };
}
