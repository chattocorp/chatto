import { serverSessionQueryRoot, type QueryKeyScope } from './keys';

function adminRoot(scope: QueryKeyScope) {
  return [...serverSessionQueryRoot(scope), 'admin'] as const;
}

export const adminQueryKeys = {
  root: adminRoot,
  registrationSettings(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'registration-settings'] as const;
  },
  membersRoot(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'members'] as const;
  },
  invitations(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'invitations'] as const;
  },
  neighbors(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'neighbors'] as const;
  },
  members(scope: QueryKeyScope, search: string) {
    return [...adminQueryKeys.membersRoot(scope), { search }] as const;
  },
  suspensions(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'suspensions'] as const;
  },
  member(scope: QueryKeyScope, userId: string) {
    return [...adminRoot(scope), 'member', userId] as const;
  },
  permissionTiers(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'permission-tier'] as const;
  },
  permissionTier(scope: QueryKeyScope, roomId: string | null, groupId: string | null) {
    return [...adminQueryKeys.permissionTiers(scope), { roomId, groupId }] as const;
  },
  rolePermissions(scope: QueryKeyScope, roleName: string) {
    return [...adminRoot(scope), 'role-permissions', roleName] as const;
  },
  userPermissionsRoot(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'user-permissions'] as const;
  },
  userPermissions(scope: QueryKeyScope, userId: string) {
    return [...adminQueryKeys.userPermissionsRoot(scope), userId] as const;
  },
  roleMembers(scope: QueryKeyScope, roleName: string) {
    return [...adminRoot(scope), 'role-members', roleName] as const;
  },
  roleCatalog(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'roles'] as const;
  },
  role(scope: QueryKeyScope, roleName: string) {
    return [...adminRoot(scope), 'role', roleName] as const;
  },
  eventLog(
    scope: QueryKeyScope,
    filter: {
      eventType: string;
      actorId: string;
      createdAtFrom: string;
      createdAtTo: string;
    }
  ) {
    return [...adminRoot(scope), 'event-log', filter] as const;
  },
  eventTypes(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'event-types'] as const;
  },
  event(scope: QueryKeyScope, sequence: string) {
    return [...adminRoot(scope), 'event', sequence] as const;
  },
  systemInfo(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'system-info'] as const;
  },
  securityConfig(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'security-config'] as const;
  },
  oauthClients(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'oauth-clients'] as const;
  },
  serverSettings(scope: QueryKeyScope) {
    return [...adminRoot(scope), 'server-settings'] as const;
  },
  room(scope: QueryKeyScope, roomId: string) {
    return [...adminRoot(scope), 'room', roomId] as const;
  },
  roomGroup(scope: QueryKeyScope, groupId: string) {
    return [...adminRoot(scope), 'room-group', groupId] as const;
  }
};
