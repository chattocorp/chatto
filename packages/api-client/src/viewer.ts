import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
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
  canAdminManageUsers: boolean;
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
};

export async function getViewerStateViaConnect(
  config: ViewerAPIConfig,
): Promise<ViewerState> {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(ViewerService, transport);
  const response = await client.getViewer(
    {},
    {
      headers: config.bearerToken
        ? { Authorization: `Bearer ${config.bearerToken}` }
        : undefined,
    },
  );
  if (!response.user) {
    throw new Error("viewer response did not include a user");
  }
  if (!response.user.profile) {
    throw new Error("viewer response did not include a user profile");
  }
  const user = response.user.profile;
  if (!user.user) {
    throw new Error("viewer response did not include a user summary");
  }
  const summary = user.user;
  const capabilities = response.capabilities;
  return {
    user: {
      id: summary.id,
      login: summary.login,
      displayName: summary.displayName,
      avatarUrl: summary.avatarUrl ?? null,
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
    canViewAdmin: capabilities?.canViewAdmin ?? false,
    canStartDMs: capabilities?.canStartDms ?? false,
    canAdminViewUsers: capabilities?.canAdminViewUsers ?? false,
    canAdminManageUsers: capabilities?.canAdminManageUsers ?? false,
    canAdminViewRoles: capabilities?.canAdminViewRoles ?? false,
    canAdminManageRoles: capabilities?.canAdminManageRoles ?? false,
    canAdminViewSystem: capabilities?.canAdminViewSystem ?? false,
    canAdminViewAudit: capabilities?.canAdminViewAudit ?? false,
    canManageUserPermissions: capabilities?.canManageUserPermissions ?? false,
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
