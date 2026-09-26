import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { Timestamp } from '@bufbuild/protobuf';
import type { UserSummary, UserPresenceView } from '$lib/api-client/userSummary';

/** Build canonical profiles for fixtures that also supply render rows. */
export function userProfileFixture(user: UserSummary & Partial<UserPresenceView>): DirectoryMember {
  return new DirectoryMember({
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
              ? Timestamp.fromDate(new Date(user.customStatus.expiresAt))
              : undefined
          }
        : undefined
    }
  });
}
