import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { MyAccountService } from '@chatto/api-types/api/v1/account_pb';

export type CustomUserStatusAPIConfig = ConnectAPIConfig & {
  serverId: string;
};

import type { CustomUserStatus } from './userSummary.js';
import { timestampFromDate, timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';

export async function setCustomStatus(
  config: CustomUserStatusAPIConfig,
  input: {
    emoji: string;
    text: string;
    expiresAt?: string | null;
  }
): Promise<CustomUserStatus | null> {
  const client = createChattoClient(MyAccountService, config);
  const response = await client.setCustomStatus({
    emoji: input.emoji,
    text: input.text,
    expiresAt: input.expiresAt ? timestampFromDate(new Date(input.expiresAt)) : undefined
  });
  return apiStatus(response.status);
}

export async function deleteCustomStatus(
  config: CustomUserStatusAPIConfig
): Promise<CustomUserStatus | null> {
  const client = createChattoClient(MyAccountService, config);
  const response = await client.deleteCustomStatus({});
  return apiStatus(response.status);
}

function apiStatus(
  status:
    | {
        emoji: string;
        text: string;
        expiresAt?: Timestamp;
      }
    | undefined
): CustomUserStatus | null {
  if (!status) return null;
  return {
    emoji: status.emoji,
    text: status.text,
    expiresAt: status.expiresAt ? timestampDate(status.expiresAt).toISOString() : null
  };
}
