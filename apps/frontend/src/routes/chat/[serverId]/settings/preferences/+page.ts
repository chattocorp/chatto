import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Redirect the former Preferences route to the named Time settings route. */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve('/chat/[serverId]/settings/time', { serverId: params.serverId })}${url.search}`
  );
};
