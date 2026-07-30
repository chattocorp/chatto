import type { DMData, RoomData } from '$lib/hooks/useRoomData.svelte';

export type RoomPresentation = {
  title: string;
  description: string | undefined;
  pageTitle: string;
};

export function buildRoomPresentation({
  roomData,
  isDM,
  dmData,
  directMessageLabel,
  currentUserLabel,
  getDisplayName
}: {
  roomData: RoomData | null | undefined;
  isDM: boolean;
  dmData: DMData | null;
  directMessageLabel: string;
  currentUserLabel: string;
  getDisplayName: (userId: string, fallback: string) => string;
}): RoomPresentation {
  if (!roomData) {
    return { title: '', description: undefined, pageTitle: '' };
  }

  if (!isDM) {
    const title = `# ${roomData.room.name}`;
    const description = roomData.room.description?.trim() || undefined;
    const pageTitle = roomData.spaceName ? `#${roomData.room.name} - ${roomData.spaceName}` : title;
    return { title, description, pageTitle };
  }

  const participants = dmData?.participants ?? [];
  const currentUserId = dmData?.currentUserId ?? null;
  const others = participants.filter((participant) => participant.id !== currentUserId);
  let title = directMessageLabel;
  if (others.length > 0) {
    title = others
      .map((participant) =>
        getDisplayName(participant.id, participant.displayName || participant.login)
      )
      .join(', ');
  } else if (participants.length > 0) {
    const self = participants.find((participant) => participant.id === currentUserId);
    title = self?.displayName || self?.login || currentUserLabel;
  }

  return { title, description: undefined, pageTitle: title };
}
