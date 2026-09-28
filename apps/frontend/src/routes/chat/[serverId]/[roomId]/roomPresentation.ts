import type { DMData, RoomData } from '$lib/hooks/useRoomData.svelte';
import { buildDirectMessagePresentation, type DirectMessageLabels } from '$lib/render/users';

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
  participantLabels,
  getDisplayName
}: {
  roomData: RoomData | null | undefined;
  isDM: boolean;
  dmData: DMData | null;
  directMessageLabel: string;
  participantLabels: DirectMessageLabels;
  getDisplayName: (userId: string, fallback: string) => string;
}): RoomPresentation {
  if (!roomData) {
    return { title: '', description: undefined, pageTitle: '' };
  }

  if (!isDM) {
    const title = `# ${roomData.room.name}`;
    const description = roomData.room.description?.trim() || undefined;
    const pageTitle = `#${roomData.room.name}`;
    return { title, description, pageTitle };
  }

  const participants = dmData?.participants ?? [];
  let title = directMessageLabel;
  if (participants.length > 0) {
    title = buildDirectMessagePresentation(
      participants,
      dmData?.currentUserId,
      participantLabels,
      getDisplayName
    ).label;
  }

  return { title, description: undefined, pageTitle: title };
}
