import type { UserAvatarUserView } from '$lib/render/users';
import type { RoomMember } from '$lib/state/room';

export class MessageUserInteractionState {
  user = $state<RoomMember | null>(null);
  anchorRect = $state<DOMRect | null>(null);

  constructor(private readonly getMembers: () => RoomMember[]) {}

  showUser(user: UserAvatarUserView | RoomMember, anchorRect: DOMRect | null): void {
    this.user =
      this.getMembers().find((candidate) => candidate.id === user.id) ??
      ({
        id: user.id,
        login: user.login,
        displayName: user.displayName,
        deleted: user.deleted ?? false,
        isBot: user.isBot,
        avatarUrl: user.avatarUrl,
        customStatus: user.customStatus,
        presenceStatus: user.presenceStatus
      } satisfies RoomMember);
    this.anchorRect = anchorRect;
  }

  hasCurrentMember(userId: string): boolean {
    return this.getMembers().some((member) => member.id === userId);
  }

  close(): void {
    this.user = null;
    this.anchorRect = null;
  }
}
