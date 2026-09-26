import { updateMask } from './updateMask';
import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { MyAccountService } from '@chatto/api-types/api/v1/account_connect';
import {
  TimeFormat,
  type UserSettings as APIUserSettings
} from '@chatto/api-types/api/v1/viewer_pb';
import { timeFormatOrAuto } from './timeFormat.js';

export type AccountUserSettings = {
  timezone?: string | null;
  timeFormat: TimeFormat;
  /** Present when the server supports private time-zone preferences. */
  shareTimezone?: boolean;
};

export type UpdateSettingsInput = {
  timezone?: string | null;
  timeFormat?: TimeFormat;
  shareTimezone?: boolean;
};

export type ChangePasswordInput = {
  password: string;
  currentPassword?: string;
};

export type VerifiedEmail = {
  email: string;
  verifiedAt: string | null;
  primary: boolean;
};

export function createAccountAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MyAccountService, config);

  return {
    async changePassword(input: ChangePasswordInput): Promise<void> {
      await client.changePassword({
        password: input.password,
        currentPassword: input.currentPassword
      });
    },

    async listVerifiedEmails(expectedUserId: string): Promise<VerifiedEmail[]> {
      const response = await client.listVerifiedEmails({ expectedUserId });
      return response.verifiedEmails.map(verifiedEmail);
    },

    async requestEmailVerification(expectedUserId: string, email: string): Promise<void> {
      await client.requestEmailVerification({ email, expectedUserId });
    },

    async confirmEmailVerification(
      expectedUserId: string,
      email: string,
      code: string
    ): Promise<VerifiedEmail[]> {
      const response = await client.confirmEmailVerification({ email, code, expectedUserId });
      return response.verifiedEmails.map(verifiedEmail);
    },

    async setPrimaryEmail(expectedUserId: string, email: string): Promise<VerifiedEmail[]> {
      const response = await client.setPrimaryEmail({ email, expectedUserId });
      return response.verifiedEmails.map(verifiedEmail);
    },

    async updateSettings(input: UpdateSettingsInput): Promise<AccountUserSettings> {
      const response = await client.updateSettings({
        timezone: input.timezone === null ? '' : input.timezone,
        timeFormat: input.timeFormat === undefined ? undefined : timeFormatOrAuto(input.timeFormat),
        shareTimezone: input.shareTimezone,
        updateMask: updateMask(input, ['timezone', 'timeFormat', 'shareTimezone'])
      });
      return userSettings(response.settings);
    },

    async requestAccountDeletion(): Promise<string> {
      return (await client.requestAccountDeletion({})).confirmationToken;
    },

    async deleteMyAccount(confirmationToken: string): Promise<boolean> {
      await client.deleteMyAccount({ confirmationToken });
      return true;
    }
  };
}

function verifiedEmail(value: {
  email: string;
  verifiedAt?: { toDate(): Date };
  primary: boolean;
}): VerifiedEmail {
  return {
    email: value.email,
    verifiedAt: value.verifiedAt?.toDate().toISOString() ?? null,
    primary: value.primary
  };
}

export type AccountAPI = ReturnType<typeof createAccountAPI>;

function userSettings(settings: APIUserSettings | undefined): AccountUserSettings {
  return {
    timezone: settings?.timezone ?? null,
    timeFormat: timeFormatOrAuto(settings?.timeFormat),
    shareTimezone: settings?.shareTimezone
  };
}
