import { resolve } from '$app/paths';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/** Keep bookmarked queue filters and individual job links usable. */
export const load: PageLoad = ({ params, url }) => {
  const destination = params.id
    ? resolve('/chat/[serverId]/manage/server/system/jobs/[id]', {
        serverId: params.serverId,
        id: params.id
      })
    : resolve('/chat/[serverId]/manage/server/system/jobs', { serverId: params.serverId });
  redirect(308, destination + url.search + url.hash);
};
