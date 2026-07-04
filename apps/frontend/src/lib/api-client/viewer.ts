import { authHeaders, createChattoClient } from "./connect.js";
import { ViewerService } from "@chatto/api-types/api/v1/viewer_connect";
import { PresenceStatus as APIPresenceStatus } from "@chatto/api-types/api/v1/presence_pb";
import { NotificationLevel as APINotificationLevel } from "@chatto/api-types/api/v1/notification_preferences_pb";
import { TimeFormat as APITimeFormat } from "@chatto/api-types/api/v1/viewer_pb";
import {
  NotificationLevel,
  PresenceStatus,
  TimeFormat,
} from "./renderTypes.js";

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
};

export type NotificationPreference = {
  level: NotificationLevel;
  effectiveLevel: NotificationLevel;
};

export type RoomNotificationPreference = NotificationPreference & {
  roomId: string;
};

export type ViewerState = ViewerCapabilities & {
  user: CurrentUser;
  serverNotificationPreference: NotificationPreference;
  roomNotificationPreferences: RoomNotificationPreference[];
  viewerPermissions: Record<string, boolean>;
  viewerHasUnreadRooms: boolean;
};

const capabilityKeys = {
  adminView: "admin.view",
  dmStart: "dm.start",
  adminViewUsers: "admin.view-users",
  adminManageAccounts: "user.manage-accounts",
  assignRoles: "role.assign",
  adminViewRoles: "role.view",
  adminManageRoles: "role.manage",
  adminViewSystem: "admin.view-system",
  adminViewAudit: "admin.view-audit",
  manageUserPermissions: "user.manage-permissions",
} as const;

export async function getViewerStateViaConnect(
  config: ViewerAPIConfig,
): Promise<ViewerState> {
  const client = createChattoClient(ViewerService, config);
  const response = await client.getViewer(
    {},
    {
      headers: authHeaders(config),
    },
  );
  if (!response.user) {
    throw new Error("viewer response did not include a user");
  }
  if (!response.user.profile) {
    throw new Error("viewer response did not include a user profile");
  }
  const user = response.user.profile;
  const grants = mapCapabilityGrants(response.capabilities?.grants);
  const viewerPermissions = mapPermissionGrants(
    response.viewerPermissions?.permissions,
  );
  const can = (capability: string) => grants[capability] ?? false;
  return {
    user: {
      id: user.id,
      login: user.login,
      displayName: user.displayName,
      avatarUrl: user.avatarUrl ?? null,
      customStatus: user.customStatus
        ? {
            emoji: user.customStatus.emoji,
            text: user.customStatus.text,
            expiresAt:
              user.customStatus.expiresAt?.toDate().toISOString() ?? null,
          }
        : null,
      presenceStatus: apiPresenceStatus(user.presenceStatus),
      hasVerifiedEmail: response.user.hasVerifiedEmail,
      hasPassword: response.user.hasPassword ?? false,
      viewerCanDeleteAccount: response.user.viewerCanDeleteAccount ?? false,
      lastLoginChange:
        response.user.lastLoginChange?.toDate().toISOString() ?? null,
      settings: response.user.settings
        ? {
            timezone: response.user.settings.timezone ?? null,
            timeFormat: apiTimeFormat(response.user.settings.timeFormat),
          }
        : null,
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
    viewerPermissions,
    viewerHasUnreadRooms: response.viewerState?.hasUnreadRooms ?? false,
    serverNotificationPreference: {
      level: apiNotificationLevel(response.serverNotificationPreference?.level),
      effectiveLevel: apiNotificationLevel(
        response.serverNotificationPreference?.effectiveLevel,
      ),
    },
    roomNotificationPreferences: response.roomNotificationPreferences.map(
      (pref) => ({
        roomId: pref.roomId,
        level: apiNotificationLevel(pref.level),
        effectiveLevel: apiNotificationLevel(pref.effectiveLevel),
      }),
    ),
  };
}

function mapPermissionGrants(
  grants: Array<{ permission: string; granted: boolean }> | undefined,
): Record<string, boolean> {
  return Object.fromEntries(
    (grants ?? []).map((grant) => [grant.permission, grant.granted]),
  );
}

function mapCapabilityGrants(
  grants: Array<{ capability: string; granted: boolean }> | undefined,
): Record<string, boolean> {
  return Object.fromEntries(
    (grants ?? []).map((grant) => [grant.capability, grant.granted]),
  );
}

export async function getCurrentUserViaConnect(
  config: ViewerAPIConfig,
): Promise<CurrentUser> {
  return (await getViewerStateViaConnect(config)).user;
}

function apiNotificationLevel(
  level: APINotificationLevel | undefined,
): NotificationLevel {
  switch (level) {
    case APINotificationLevel.MUTED:
      return NotificationLevel.Muted;
    case APINotificationLevel.NORMAL:
      return NotificationLevel.Normal;
    case APINotificationLevel.ALL_MESSAGES:
      return NotificationLevel.AllMessages;
    case APINotificationLevel.DEFAULT:
    case APINotificationLevel.UNSPECIFIED:
    default:
      return NotificationLevel.Default;
  }
}

function apiPresenceStatus(status: APIPresenceStatus): PresenceStatus {
  switch (status) {
    case APIPresenceStatus.AWAY:
      return PresenceStatus.Away;
    case APIPresenceStatus.DO_NOT_DISTURB:
      return PresenceStatus.DoNotDisturb;
    case APIPresenceStatus.ONLINE:
      return PresenceStatus.Online;
    case APIPresenceStatus.OFFLINE:
    case APIPresenceStatus.UNSPECIFIED:
    default:
      return PresenceStatus.Offline;
  }
}

function apiTimeFormat(format: APITimeFormat): TimeFormat {
  switch (format) {
    case APITimeFormat.TIME_FORMAT_12_HOUR:
      return TimeFormat.TwelveHour;
    case APITimeFormat.TIME_FORMAT_24_HOUR:
      return TimeFormat.TwentyFourHour;
    case APITimeFormat.TIME_FORMAT_AUTO:
    case APITimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}
