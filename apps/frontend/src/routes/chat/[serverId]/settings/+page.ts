import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { DEFAULT_SERVER_SETTINGS_ROUTE } from '$lib/navigation/settingsRoutes';
import type { PageLoad } from './$types';

/** Redirect the canonical Settings entry point to its first displayed page. */
export const load: PageLoad = ({ params, url }) => {
  redirect(
    308,
    `${resolve(DEFAULT_SERVER_SETTINGS_ROUTE, { serverId: params.serverId })}${url.search}`
  );
};
