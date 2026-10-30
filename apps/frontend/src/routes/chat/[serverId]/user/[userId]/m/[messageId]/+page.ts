import { navigationVisits } from '$lib/navigation/mutationCompletion';
import { serverConnectionManager, serverRegistry } from '$lib/client';
import { segmentToServerId } from '$lib/navigation';
import { userPath } from '$lib/navigation/conversationPath';
import { resolveAndRedirect } from '$lib/navigation/resolveMessageDestination';
import { serverUi } from '$lib/state/server/serverUi';
import { createRoomTimelineAPI } from '@chatto/client/api/roomTimeline';
import type { PageLoad } from './$types';

/** Resolve after conversation readiness; a late lookup must not redirect another destination. */
export const load: PageLoad = async ({ params, parent }) => {
  const visitCurrent = navigationVisits.capture();
  const { conversationRoomId } = await parent();
  const serverId = segmentToServerId(params.serverId);
  const userId = params.userId;
  const messageId = params.messageId;
  const store = serverId ? serverRegistry.tryGetStore(serverId) : undefined;
  if (!serverId || !store) return {};
  const isCurrent = () => visitCurrent() && serverRegistry.tryGetStore(serverId) === store;
  void conversationRoomId.then((roomId) => {
    if (!roomId || !isCurrent()) return;
    return resolveAndRedirect(
      serverConnectionManager.getClient(serverId).getAPI(createRoomTimelineAPI),
      serverUi(store).pendingHighlights,
      (threadId) => userPath(serverId, userId, { threadId }),
      roomId,
      messageId,
      isCurrent
    );
  });
  return {};
};
