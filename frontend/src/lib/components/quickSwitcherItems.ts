export type QuickSwitcherKind = 'room' | 'dm' | 'destination' | 'server' | 'user';

export type QuickSwitcherParticipant = {
  id: string;
};

export type QuickSwitcherItemBase = {
  kind: QuickSwitcherKind;
  serverId: string;
  participants?: QuickSwitcherParticipant[];
  currentUserId?: string;
  targetUserId?: string;
};

function oneToOneOrSelfDmTarget(item: QuickSwitcherItemBase): string | null {
  if (item.kind !== 'dm' || !item.currentUserId || !item.participants) return null;

  const others = item.participants.filter((p) => p.id !== item.currentUserId);
  if (others.length === 0) return item.currentUserId;
  if (others.length === 1) return others[0].id;
  return null;
}

function userKey(item: Pick<QuickSwitcherItemBase, 'serverId' | 'targetUserId'>): string | null {
  return item.targetUserId ? `${item.serverId}:${item.targetUserId}` : null;
}

export function mergeQuickSwitcherUserResults<T extends QuickSwitcherItemBase>(
  baseItems: T[],
  userItems: T[]
): T[] {
  if (userItems.length === 0) return baseItems;

  const userKeys = new Set(
    userItems.map((item) => userKey(item)).filter((key): key is string => key !== null)
  );

  const dedupedBase = baseItems.filter((item) => {
    const dmTarget = oneToOneOrSelfDmTarget(item);
    return !dmTarget || !userKeys.has(`${item.serverId}:${dmTarget}`);
  });

  return [...dedupedBase, ...userItems];
}
