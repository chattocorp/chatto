import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';

type AdminQueryConnection = Pick<ServerConnection, 'queryScope'>;

function adminRoot(serverId: string, connection: AdminQueryConnection) {
  return ['server', serverId, 'session', connection.queryScope, 'admin'] as const;
}

function permissionTiersRoot(serverId: string, connection: AdminQueryConnection) {
  return [...adminRoot(serverId, connection), 'permission-tier'] as const;
}

function userPermissionsRoot(serverId: string, connection: AdminQueryConnection) {
  return [...adminRoot(serverId, connection), 'user-permissions'] as const;
}

export const adminQueryKeys = {
  root: adminRoot,
  membersRoot(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'members'] as const;
  },
  invitations(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'invitations'] as const;
  },
  members(serverId: string, connection: AdminQueryConnection, search: string) {
    return [...adminQueryKeys.membersRoot(serverId, connection), { search }] as const;
  },
  bansRoot(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'bans'] as const;
  },
  bans(serverId: string, connection: AdminQueryConnection) {
    return adminQueryKeys.bansRoot(serverId, connection);
  },
  member(serverId: string, connection: AdminQueryConnection, userId: string) {
    return [...adminRoot(serverId, connection), 'member', userId] as const;
  },
  permissionTiers(serverId: string, connection: AdminQueryConnection) {
    return permissionTiersRoot(serverId, connection);
  },
  permissionTier(
    serverId: string,
    connection: AdminQueryConnection,
    roomId: string | null,
    groupId: string | null
  ) {
    return [...permissionTiersRoot(serverId, connection), { roomId, groupId }] as const;
  },
  rolePermissions(serverId: string, connection: AdminQueryConnection, roleName: string) {
    return [...adminRoot(serverId, connection), 'role-permissions', roleName] as const;
  },
  userPermissionsRoot,
  userPermissions(serverId: string, connection: AdminQueryConnection, userId: string) {
    return [...userPermissionsRoot(serverId, connection), userId] as const;
  },
  roleCatalog(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'roles'] as const;
  },
  role(serverId: string, connection: AdminQueryConnection, roleName: string) {
    return [...adminRoot(serverId, connection), 'role', roleName] as const;
  },
  eventLog(
    serverId: string,
    connection: AdminQueryConnection,
    filter: {
      eventType: string;
      actorId: string;
      createdAtFrom: string;
      createdAtTo: string;
    }
  ) {
    return [...adminRoot(serverId, connection), 'event-log', filter] as const;
  },
  eventTypes(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'event-types'] as const;
  },
  event(serverId: string, connection: AdminQueryConnection, sequence: string) {
    return [...adminRoot(serverId, connection), 'event', sequence] as const;
  },
  systemInfo(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'system-info'] as const;
  },
  securityConfig(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'security-config'] as const;
  },
  oauthClients(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'oauth-clients'] as const;
  },
  serverSettings(serverId: string, connection: AdminQueryConnection) {
    return [...adminRoot(serverId, connection), 'server-settings'] as const;
  },
  room(serverId: string, connection: AdminQueryConnection, roomId: string) {
    return [...adminRoot(serverId, connection), 'room', roomId] as const;
  },
  roomGroup(serverId: string, connection: AdminQueryConnection, groupId: string) {
    return [...adminRoot(serverId, connection), 'room-group', groupId] as const;
  }
};
