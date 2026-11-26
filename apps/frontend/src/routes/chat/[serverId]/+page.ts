import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { dev } from '$app/environment';
import { handoffHost } from '$lib/serverUrl';
import { segmentToServerId, serverIdToSegment } from '$lib/navigation';
import { getLastRoom } from '$lib/storage/lastRoom';
import type { PageLoad } from './$types';

/** Open a saved server, or show the consent card before discovering a new one. */
export const load: PageLoad = async ({ params, url, parent }) => {
  // The root layout restores saved registrations before lookup.
  await parent();
  const host = handoffHost(params.serverId, dev);
  const serverId = segmentToServerId(host?.hostname ?? params.serverId);
  if (!serverId) {
    if (params.serverId === '-') redirect(302, resolve('/login'));
    return { origin: host?.origin ?? null };
  }
  const segment = serverIdToSegment(serverId);

  const lastRoomId = getLastRoom(serverId);
  if (lastRoomId) {
    redirect(
      302,
      `${resolve('/chat/[serverId]/[roomId]', {
        serverId: segment,
        roomId: lastRoomId
      })}${url.search}`
    );
  }

  redirect(302, `${resolve('/chat/[serverId]/overview', { serverId: segment })}${url.search}`);
};
