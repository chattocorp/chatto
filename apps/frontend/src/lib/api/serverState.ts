import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';

export type ServerStateAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

export type AuthenticatedServerState = {
  name: string;
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
  viewerHasAnyAdminPermission: boolean;
  viewerCanManageServer: boolean;
  viewerCanCreateRoom: boolean;
  viewerCanManageRooms: boolean;
  viewerHasUnreadRooms: boolean;
};

export type EditableServerConfig = {
  name: string;
  description: string;
  motd: string;
  welcomeMessage: string;
};

export type EditableServerProfile = {
  name: string;
  description: string | null;
  motd: string | null;
  welcomeMessage: string | null;
  logoUrl: string | null;
  bannerUrl: string | null;
};

export type ServerSecurityConfig = {
  blockedUsernames: string;
};

function serverClients(config: ServerStateAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const server = createClient(ServerService, transport);
  const adminServer = createClient(AdminServerService, transport);
  const headers = config.bearerToken
    ? { Authorization: `Bearer ${config.bearerToken}` }
    : undefined;
  return { server, adminServer, headers };
}

export async function getAuthenticatedServerState(
  config: ServerStateAPIConfig
): Promise<AuthenticatedServerState> {
  const { server, headers } = serverClients(config);
  const response = await server.getServerState({}, { headers });

  return {
    name: response.profile?.name || 'Chatto',
    logoUrl: response.profile?.logoUrl ?? null,
    bannerUrl: response.profile?.bannerUrl ?? null,
    welcomeMessage: response.profile?.welcomeMessage ?? null,
    description: response.profile?.description ?? null,
    motd: response.profile?.motd ?? null,
    pushNotificationsEnabled: response.pushNotificationsEnabled,
    vapidPublicKey: response.vapidPublicKey ?? null,
    livekitUrl: response.livekitUrl ?? null,
    videoProcessingEnabled: response.videoProcessingEnabled,
    maxUploadSize: Number(response.maxUploadSize),
    maxVideoUploadSize: Number(response.maxVideoUploadSize),
    messageEditWindowSeconds: response.messageEditWindowSeconds,
    viewerHasAnyAdminPermission: response.viewerCapabilities?.hasAnyAdminPermission ?? false,
    viewerCanManageServer: response.viewerCapabilities?.canManageServer ?? false,
    viewerCanCreateRoom: response.viewerCapabilities?.canCreateRoom ?? false,
    viewerCanManageRooms: response.viewerCapabilities?.canManageRooms ?? false,
    viewerHasUnreadRooms: response.viewerCapabilities?.hasUnreadRooms ?? false
  };
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

  return editableServerProfile(response.profile);
}

export async function uploadServerLogo(
  config: ServerStateAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerLogo(
    {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type
    },
    { headers }
  );
  return editableServerProfile(response.profile);
}

export async function deleteServerLogo(
  config: ServerStateAPIConfig
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerLogo({}, { headers });
  return editableServerProfile(response.profile);
}

export async function uploadServerBanner(
  config: ServerStateAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerBanner(
    {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type
    },
    { headers }
  );
  return editableServerProfile(response.profile);
}

export async function deleteServerBanner(
  config: ServerStateAPIConfig
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerBanner({}, { headers });
  return editableServerProfile(response.profile);
}

export async function getServerSecurityConfig(
  config: ServerStateAPIConfig
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.getServerSecurityConfig({}, { headers });
  return {
    blockedUsernames: response.blockedUsernames
  };
}

export async function updateBlockedUsernames(
  config: ServerStateAPIConfig,
  blockedUsernames: string
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.updateBlockedUsernames({ blockedUsernames }, { headers });
  return {
    blockedUsernames: response.blockedUsernames
  };
}

function editableServerProfile(
  profile: {
    name?: string;
    logoUrl?: string;
    bannerUrl?: string;
    welcomeMessage?: string;
    description?: string;
    motd?: string;
  } | null | undefined
): EditableServerProfile {
  return {
    name: profile?.name || 'Chatto',
    logoUrl: profile?.logoUrl ?? null,
    bannerUrl: profile?.bannerUrl ?? null,
    welcomeMessage: profile?.welcomeMessage ?? null,
    description: profile?.description ?? null,
    motd: profile?.motd ?? null
  };
}
