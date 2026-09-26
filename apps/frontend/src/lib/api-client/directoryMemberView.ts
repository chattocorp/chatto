import type { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import {
  mapUserPresenceView,
  mapUserSummary,
  type UserPresenceView,
  type UserSummary
} from './userSummary';

export type DirectoryMember = UserSummary &
  UserPresenceView & {
    roles: string[];
    createdAt: string | null;
  };

/** Map the canonical public profile to a render snapshot at the API/view boundary. */
export function mapDirectoryMember(member: APIDirectoryMember): DirectoryMember {
  const user = member.user;
  const summary: UserSummary = user
    ? { ...mapUserSummary(user), isBot: !!user.bot }
    : { id: '', login: '', displayName: '', deleted: false, isBot: false, avatarUrl: null };
  return {
    ...summary,
    ...mapUserPresenceView(user),
    roles: [...member.roles],
    createdAt: member.createdAt?.toDate().toISOString() ?? null
  };
}
