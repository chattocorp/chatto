import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Redirect the internal `app` route to the named Appearance settings route. */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve('/chat/[serverId]/settings/appearance', { serverId: params.serverId })}${url.search}`
  );
};
