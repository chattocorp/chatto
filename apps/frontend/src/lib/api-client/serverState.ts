import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { AdminServerService } from '@chatto/api-types/admin/v1/server_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { mapServerProfile, type ServerProfile } from './serverProfile.js';

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
  viewerPermissions: Record<string, boolean>;
  viewerCanManageServer: boolean;
  viewerCanCreateRooms: boolean;
  viewerCanJoinRooms: boolean;
  viewerCanListRooms: boolean;
  viewerCanManageRooms: boolean;
  viewerCanBanRoomMembers: boolean;
  viewerCanPostMessages: boolean;
  viewerCanPostInThreads: boolean;
  viewerCanAttachFiles: boolean;
  viewerCanManageMessages: boolean;
  viewerCanReactToMessages: boolean;
  viewerCanEchoMessages: boolean;
  viewerCanManageRoles: boolean;
  viewerCanAssignRoles: boolean;
  viewerCanViewAdminUsers: boolean;
  viewerCanViewAdminSystem: boolean;
  viewerCanViewAdminAudit: boolean;
  viewerCanDeleteAnyUser: boolean;
  viewerCanDeleteSelf: boolean;
  viewerCanManageUserPermissions: boolean;
  viewerHasUnreadRooms: boolean;
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

function mapViewerPermissions(
  permissions: Array<{ permission: string; granted: boolean }> | undefined
): Record<string, boolean> {
  return Object.fromEntries(
    (permissions ?? []).map((permission) => [permission.permission, permission.granted])
  );
}

function mapViewerCapabilities(
  capabilities: Array<{ capability: string; granted: boolean }> | undefined
): Record<string, boolean> {
  return Object.fromEntries(
    (capabilities ?? []).map((capability) => [capability.capability, capability.granted])
  );
}

function serverClients(config: ConnectAPIConfig) {
  const discovery = createChattoClient(ServerDiscoveryService, config);
  const server = createChattoClient(ServerService, config);
  const viewer = createChattoClient(ViewerService, config);
  const adminServer = createChattoClient(AdminServerService, config);
  return { discovery, server, viewer, adminServer };
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
  config: ConnectAPIConfig,
  options: { signal?: AbortSignal } = {}
): Promise<AuthenticatedServerState> {
  const { discovery, server, viewer } = serverClients(config);
  const authenticatedCallOptions = { signal: options.signal };
  const [discoveryResponse, motdResponse, runtimeResponse, viewerResponse] = await Promise.all([
    options.signal ? discovery.getServer({}, { signal: options.signal }) : discovery.getServer({}),
    server.getMotd({}, authenticatedCallOptions),
    server.getRuntimeConfig({}, authenticatedCallOptions),
    viewer.getViewer({}, authenticatedCallOptions)
  ]);
  const profile = mapServerProfile(discoveryResponse.profile);
  const runtime = runtimeResponse.runtime;
  const viewerPermissions = mapViewerPermissions(viewerResponse.viewerPermissions?.permissions);
  const viewerCapabilities = mapViewerCapabilities(viewerResponse.capabilities?.grants);
  const viewerState = viewerResponse.viewerState;
  const can = (permission: string) => viewerPermissions[permission] ?? false;
  const capability = (key: string) => viewerCapabilities[key] ?? false;

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
    messageEditWindowSeconds: runtime?.messageEditWindowSeconds ?? 0,
    viewerPermissions,
    viewerCanManageServer: can('server.manage'),
    viewerCanCreateRooms: can('room.create'),
    viewerCanJoinRooms: can('room.join'),
    viewerCanListRooms: can('room.list'),
    viewerCanManageRooms: can('room.manage'),
    viewerCanBanRoomMembers: can('room.remove-member'),
    viewerCanPostMessages: can('message.post'),
    viewerCanPostInThreads: can('message.post-in-thread'),
    viewerCanAttachFiles: can('message.attach'),
    viewerCanManageMessages: can('message.manage'),
    viewerCanReactToMessages: can('message.react'),
    viewerCanEchoMessages: can('message.echo'),
    viewerCanManageRoles: can('role.manage'),
    viewerCanAssignRoles: can('role.assign'),
    viewerCanViewAdminUsers: can('admin.view-users'),
    viewerCanViewAdminSystem: capability('admin.view-system'),
    viewerCanViewAdminAudit: can('admin.view-audit'),
    viewerCanDeleteAnyUser: can('user.delete-any'),
    viewerCanDeleteSelf: can('user.delete-self'),
    viewerCanManageUserPermissions: can('user.manage-permissions'),
    viewerHasUnreadRooms: viewerState?.hasUnreadRooms ?? false
  };
}

export async function getServerConfig(
  config: ConnectAPIConfig,
  options: { signal?: AbortSignal } = {}
): Promise<EditableServerConfig> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.getServerConfig({}, { signal: options.signal });
  return mapEditableServerConfig(response.config);
}

export async function updateServerConfig(
  config: ConnectAPIConfig,
  input: EditableServerConfig
): Promise<EditableServerProfile> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.updateServerConfig({
    serverName: input.name,
    description: input.description,
    motd: input.motd,
    welcomeMessage: input.welcomeMessage,
    updateMask: { paths: ['server_name', 'description', 'motd', 'welcome_message'] }
  });

  return mapServerProfile({ publicProfile: response.publicProfile, motd: response.config?.motd });
}

export async function uploadServerLogo(
  config: ConnectAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.uploadServerLogo({
    image: {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type
    }
  });
  return mapServerProfile(response.publicProfile);
}

export async function deleteServerLogo(config: ConnectAPIConfig): Promise<EditableServerProfile> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.deleteServerLogo({});
  return mapServerProfile(response.publicProfile);
}

export async function uploadServerBanner(
  config: ConnectAPIConfig,
  file: File
): Promise<EditableServerProfile> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.uploadServerBanner({
    image: {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type
    }
  });
  return mapServerProfile(response.publicProfile);
}

export async function deleteServerBanner(config: ConnectAPIConfig): Promise<EditableServerProfile> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.deleteServerBanner({});
  return mapServerProfile(response.publicProfile);
}

export async function getServerSecurityConfig(
  config: ConnectAPIConfig,
  options: { signal?: AbortSignal } = {}
): Promise<ServerSecurityConfig> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.getServerSecurityConfig({}, { signal: options.signal });
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames)
  };
}

export async function updateBlockedUsernames(
  config: ConnectAPIConfig,
  blockedUsernames: string
): Promise<ServerSecurityConfig> {
  const { adminServer } = serverClients(config);
  const response = await adminServer.updateBlockedUsernames({
    blockedUsernames: blockedUsernameEntries(blockedUsernames),
    updateMask: { paths: ['blocked_usernames'] }
  });
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames)
  };
}
