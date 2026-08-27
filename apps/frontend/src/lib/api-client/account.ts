import { authHeaders, createChattoClient } from './connect.js';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import {
  TimeFormat,
  type UserSettings as APIUserSettings
} from '@chatto/api-types/api/v1/viewer_pb';
import { timeFormatOrAuto } from './timeFormat.js';

export type AccountAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type AccountUser = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string | null;
};

export type AccountUserSettings = {
  timezone?: string | null;
  timeFormat: TimeFormat;
};

export type UpdateProfileInput = {
  displayName?: string;
  login?: string;
};

export type UpdateSettingsInput = {
  timezone?: string | null;
  timeFormat?: TimeFormat;
};

export type UpdatePasswordInput = {
  password: string;
  currentPassword?: string;
};

export function createAccountAPI(config: AccountAPIConfig) {
  const client = createChattoClient(MyAccountService, config);
  const headers = () => authHeaders(config);

  return {
    async updateProfile(input: UpdateProfileInput): Promise<AccountUser> {
      const response = await client.updateProfile(input, {
        headers: headers()
      });
      return accountUser(response.user);
    },

    async uploadAvatar(file: File): Promise<AccountUser> {
      const response = await client.uploadAvatar(
        {
          image: {
            image: new Uint8Array(await file.arrayBuffer()),
            filename: file.name,
            contentType: file.type
          }
        },
        { headers: headers() }
      );
      return accountUser(response.user);
    },

    async deleteAvatar(): Promise<AccountUser> {
      const response = await client.deleteAvatar({}, { headers: headers() });
      return accountUser(response.user);
    },

    async updatePassword(input: UpdatePasswordInput): Promise<void> {
      await client.updatePassword(
        { password: input.password, currentPassword: input.currentPassword },
        { headers: headers() }
      );
    },

    async updateSettings(input: UpdateSettingsInput): Promise<AccountUserSettings> {
      const response = await client.updateSettings(
        {
          timezone: input.timezone === null ? '' : input.timezone,
          timeFormat:
            input.timeFormat === undefined ? undefined : timeFormatOrAuto(input.timeFormat)
        },
        { headers: headers() }
      );
      return userSettings(response.settings);
    },

    async requestAccountDeletion(): Promise<string> {
      return (await client.requestAccountDeletion({}, { headers: headers() })).confirmationToken;
    },

    async deleteMyAccount(confirmationToken: string): Promise<boolean> {
      return (
        await client.deleteMyAccount(
          { confirmationToken },
          {
            headers: headers()
          }
        )
      ).deleted;
    }
  };
}

export type AccountAPI = ReturnType<typeof createAccountAPI>;

function accountUser(user: APIUser | undefined): AccountUser {
  if (!user) {
    throw new Error('account response did not include a user');
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null
  };
}

function userSettings(settings: APIUserSettings | undefined): AccountUserSettings {
  return {
    timezone: settings?.timezone ?? null,
    timeFormat: timeFormatOrAuto(settings?.timeFormat)
  };
}
