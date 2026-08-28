import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Redirect the former settings root to the named Profile settings route. */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve('/chat/[serverId]/settings/profile', { serverId: params.serverId })}${url.search}`
  );
};
