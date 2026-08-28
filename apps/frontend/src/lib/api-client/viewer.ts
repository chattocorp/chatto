import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { authHeaders, createChattoClient } from './connect.js';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { TimeFormat, type GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import { presenceStatusOrOffline } from './enumDefaults.js';
import { timeFormatOrAuto } from './timeFormat.js';

export type ViewerAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type CurrentUser = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string | null;
  bio?: string | null;
  customStatus?: {
    emoji: string;
    text: string;
    expiresAt?: string | null;
  } | null;
  presenceStatus: PresenceStatus;
  hasVerifiedEmail: boolean;
  hasPassword: boolean;
  viewerCanDeleteAccount: boolean;
  lastLoginChange?: string | null;
  settings?: {
    timezone?: string | null;
    timeFormat: TimeFormat;
  } | null;
};

export type ViewerCapabilities = {
  canViewAdmin: boolean;
  canStartDMs: boolean;
  canAdminViewUsers: boolean;
  canAdminManageAccounts: boolean;
  canAssignRoles: boolean;
  canAdminViewRoles: boolean;
  canAdminManageRoles: boolean;
  canAdminViewSystem: boolean;
  canAdminViewAudit: boolean;
  canManageUserPermissions: boolean;
  canManageInvites: boolean;
};

export type ViewerState = ViewerCapabilities & {
  user: CurrentUser;
  viewerPermissions: Record<string, boolean>;
  viewerHasUnreadRooms: boolean;
};

const capabilityKeys = {
  adminView: 'admin.view',
  dmStart: 'dm.start',
  adminViewUsers: 'admin.view-users',
  adminManageAccounts: 'user.manage-accounts',
  assignRoles: 'role.assign',
  adminViewRoles: 'role.view',
  adminManageRoles: 'role.manage',
  adminViewSystem: 'admin.view-system',
  adminViewAudit: 'admin.view-audit',
  manageUserPermissions: 'user.manage-permissions',
  manageInvites: 'user.invite'
} as const;

export async function getViewerStateViaConnect(
  config: ViewerAPIConfig,
  options: { signal?: AbortSignal } = {}
): Promise<ViewerState> {
  const client = createChattoClient(ViewerService, config);
  const response = await client.getViewer(
    {},
    {
      headers: authHeaders(config),
      ...(options.signal ? { signal: options.signal } : {})
    }
  );
  return viewerResponseToState(response);
}

export function viewerResponseToState(response: GetViewerResponse): ViewerState {
  if (!response.user) {
    throw new Error('viewer response did not include a user');
  }
  if (!response.user.profile) {
    throw new Error('viewer response did not include a user profile');
  }
  const user = response.user.profile;
  const grants = mapCapabilityGrants(response.capabilities?.grants);
  const viewerPermissions = mapPermissionGrants(response.viewerPermissions?.permissions);
  const can = (capability: string) => grants[capability] ?? false;
  return {
    user: {
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      avatarUrl: user.avatarUrl ?? null,
      bio: user.bio ?? null,
      customStatus: user.customStatus
        ? {
            emoji: user.customStatus.emoji,
            text: user.customStatus.text,
            expiresAt: user.customStatus.expiresAt?.toDate().toISOString() ?? null
          }
        : null,
      presenceStatus: presenceStatusOrOffline(user.presenceStatus),
      hasVerifiedEmail: response.user.hasVerifiedEmail,
      hasPassword: response.user.hasPassword ?? false,
      viewerCanDeleteAccount: response.user.viewerCanDeleteAccount ?? false,
      lastLoginChange: response.user.lastLoginChange?.toDate().toISOString() ?? null,
      settings: response.user.settings
        ? {
            timezone: response.user.settings.timezone ?? null,
            timeFormat: timeFormatOrAuto(response.user.settings.timeFormat)
          }
        : null
    },
    canViewAdmin: can(capabilityKeys.adminView),
    canStartDMs: can(capabilityKeys.dmStart),
    canAdminViewUsers: can(capabilityKeys.adminViewUsers),
    canAdminManageAccounts: can(capabilityKeys.adminManageAccounts),
    canAssignRoles: can(capabilityKeys.assignRoles),
    canAdminViewRoles: can(capabilityKeys.adminViewRoles),
    canAdminManageRoles: can(capabilityKeys.adminManageRoles),
    canAdminViewSystem: can(capabilityKeys.adminViewSystem),
    canAdminViewAudit: can(capabilityKeys.adminViewAudit),
    canManageUserPermissions: can(capabilityKeys.manageUserPermissions),
    canManageInvites: can(capabilityKeys.manageInvites),
    viewerPermissions,
    viewerHasUnreadRooms: response.viewerState?.hasUnreadRooms ?? false
  };
}

function mapPermissionGrants(
  grants: Array<{ permission: string; granted: boolean }> | undefined
): Record<string, boolean> {
  return Object.fromEntries((grants ?? []).map((grant) => [grant.permission, grant.granted]));
}

function mapCapabilityGrants(
  grants: Array<{ capability: string; granted: boolean }> | undefined
): Record<string, boolean> {
  return Object.fromEntries((grants ?? []).map((grant) => [grant.capability, grant.granted]));
}

export async function getCurrentUserViaConnect(config: ViewerAPIConfig): Promise<CurrentUser> {
  return (await getViewerStateViaConnect(config)).user;
}
