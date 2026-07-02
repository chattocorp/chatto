import { authHeaders, createChattoClient } from "./connect.js";
import { AdminServerService } from "@chatto/api-types/admin/v1/server_connect";
import { ServerService } from "@chatto/api-types/api/v1/server_state_connect";
import { ViewerService } from "@chatto/api-types/api/v1/viewer_connect";
import { mapServerProfile, type ServerProfile } from "./serverProfile.js";

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
  permissions: Array<{ permission: string; granted: boolean }> | undefined,
): Record<string, boolean> {
  return Object.fromEntries(
    (permissions ?? []).map((permission) => [
      permission.permission,
      permission.granted,
    ]),
  );
}

function serverClients(config: ServerStateAPIConfig) {
  const server = createChattoClient(ServerService, config);
  const viewer = createChattoClient(ViewerService, config);
  const adminServer = createChattoClient(AdminServerService, config);
  const headers = authHeaders(config);
  return { server, viewer, adminServer, headers };
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
    | undefined,
): EditableServerConfig {
  return {
    name: config?.serverName ?? "",
    description: config?.description ?? "",
    motd: config?.motd ?? "",
    welcomeMessage: config?.welcomeMessage ?? "",
  };
}

function blockedUsernamesText(entries: readonly string[] | undefined): string {
  return (entries ?? []).join("\n");
}

function blockedUsernameEntries(text: string): string[] {
  return text
    .split("\n")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export async function getAuthenticatedServerState(
  config: ServerStateAPIConfig,
): Promise<AuthenticatedServerState> {
  const { server, viewer, headers } = serverClients(config);
  const [response, viewerResponse] = await Promise.all([
    server.getServerState({}, { headers }),
    viewer.getViewer({}, { headers }),
  ]);
  const profile = mapServerProfile(response.profile);
  const runtime = response.runtime;
  const viewerPermissions = mapViewerPermissions(
    viewerResponse.viewerPermissions?.permissions,
  );
  const viewerState = viewerResponse.viewerState;
  const can = (permission: string) => viewerPermissions[permission] ?? false;

  return {
    name: profile.name,
    version: profile.version,
    logoUrl: profile.logoUrl,
    bannerUrl: profile.bannerUrl,
    welcomeMessage: profile.welcomeMessage,
    description: profile.description,
    motd: profile.motd,
    pushNotificationsEnabled: runtime?.pushNotificationsEnabled ?? false,
    vapidPublicKey: runtime?.vapidPublicKey ?? null,
    livekitUrl: runtime?.livekitUrl ?? null,
    videoProcessingEnabled: runtime?.videoProcessingEnabled ?? false,
    maxUploadSize: Number(runtime?.maxUploadSize ?? 0),
    maxVideoUploadSize: Number(runtime?.maxVideoUploadSize ?? 0),
    messageEditWindowSeconds: runtime?.messageEditWindowSeconds ?? 0,
    viewerPermissions,
    viewerCanManageServer: can("server.manage"),
    viewerCanCreateRooms: can("room.create"),
    viewerCanJoinRooms: can("room.join"),
    viewerCanListRooms: can("room.list"),
    viewerCanManageRooms: can("room.manage"),
    viewerCanBanRoomMembers: can("room.ban-member"),
    viewerCanPostMessages: can("message.post"),
    viewerCanPostInThreads: can("message.post-in-thread"),
    viewerCanAttachFiles: can("message.attach"),
    viewerCanManageMessages: can("message.manage"),
    viewerCanReactToMessages: can("message.react"),
    viewerCanEchoMessages: can("message.echo"),
    viewerCanManageRoles: can("role.manage"),
    viewerCanAssignRoles: can("role.assign"),
    viewerCanViewAdminUsers: can("admin.view-users"),
    viewerCanViewAdminSystem: can("admin.view-system"),
    viewerCanViewAdminAudit: can("admin.view-audit"),
    viewerCanDeleteAnyUser: can("user.delete-any"),
    viewerCanDeleteSelf: can("user.delete-self"),
    viewerCanManageUserPermissions: can("user.manage-permissions"),
    viewerHasUnreadRooms: viewerState?.hasUnreadRooms ?? false,
  };
}

export async function getServerConfig(
  config: ServerStateAPIConfig,
): Promise<EditableServerConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.getServerConfig({}, { headers });
  return mapEditableServerConfig(response.config);
}

export async function updateServerConfig(
  config: ServerStateAPIConfig,
  input: EditableServerConfig,
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.updateServerConfig(
    {
      serverName: input.name,
      description: input.description,
      motd: input.motd,
      welcomeMessage: input.welcomeMessage,
    },
    { headers },
  );

  return mapServerProfile(response.profile);
}

export async function uploadServerLogo(
  config: ServerStateAPIConfig,
  file: File,
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerLogo(
    {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type,
    },
    { headers },
  );
  return mapServerProfile(response.profile);
}

export async function deleteServerLogo(
  config: ServerStateAPIConfig,
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerLogo({}, { headers });
  return mapServerProfile(response.profile);
}

export async function uploadServerBanner(
  config: ServerStateAPIConfig,
  file: File,
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.uploadServerBanner(
    {
      image: new Uint8Array(await file.arrayBuffer()),
      filename: file.name,
      contentType: file.type,
    },
    { headers },
  );
  return mapServerProfile(response.profile);
}

export async function deleteServerBanner(
  config: ServerStateAPIConfig,
): Promise<EditableServerProfile> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.deleteServerBanner({}, { headers });
  return mapServerProfile(response.profile);
}

export async function getServerSecurityConfig(
  config: ServerStateAPIConfig,
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.getServerSecurityConfig({}, { headers });
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames),
  };
}

export async function updateBlockedUsernames(
  config: ServerStateAPIConfig,
  blockedUsernames: string,
): Promise<ServerSecurityConfig> {
  const { adminServer, headers } = serverClients(config);
  const response = await adminServer.updateBlockedUsernames(
    { blockedUsernames: blockedUsernameEntries(blockedUsernames) },
    { headers },
  );
  return {
    blockedUsernames: blockedUsernamesText(response.blockedUsernames),
  };
}
