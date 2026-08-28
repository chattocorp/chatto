import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { segmentToServerId } from '$lib/navigation';
import { getLastRoom } from '$lib/storage/lastRoom';
import type { PageLoad } from './$types';

/** Redirect a server root to its remembered room, or to the server overview. */
export const load: PageLoad = ({ params, url }) => {
  const serverId = segmentToServerId(params.serverId);
  if (!serverId) redirect(302, resolve('/login'));

  const lastRoomId = getLastRoom(serverId);
  if (lastRoomId) {
    redirect(
      302,
      `${resolve('/chat/[serverId]/[roomId]', {
        serverId: params.serverId,
        roomId: lastRoomId
      })}${url.search}`
    );
  }

  redirect(302, `${resolve('/chat/[serverId]/overview', { serverId: params.serverId })}${url.search}`);
};
