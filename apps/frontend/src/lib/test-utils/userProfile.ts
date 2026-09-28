import {
  type DirectoryMember,
  DirectoryMemberSchema
} from '@chatto/api-types/api/v1/member_directory_pb';
import type { UserSummary, UserPresenceView } from '$lib/api-client/userSummary';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { create } from '@bufbuild/protobuf';

/** Build canonical profiles for fixtures that also supply render rows. */
export function userProfileFixture(user: UserSummary & Partial<UserPresenceView>): DirectoryMember {
  return create(DirectoryMemberSchema, {
    user: {
      ...user,
      avatarUrl: user.avatarUrl ?? '',
      bio: user.bio ?? '',
      timezone: user.timezone ?? '',
      bot: user.bot ?? (user.isBot ? { ownerUserId: '' } : undefined),
      customStatus: user.customStatus
        ? {
            ...user.customStatus,
            expiresAt: user.customStatus.expiresAt
              ? timestampFromDate(new Date(user.customStatus.expiresAt))
              : undefined
          }
        : undefined
    }
  });
}
