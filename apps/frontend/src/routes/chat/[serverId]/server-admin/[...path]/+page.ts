import { error, redirect } from '@sveltejs/kit';
import { legacyServerAdminDestination } from '$lib/navigation/legacyServerAdminRedirect';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params, url }) => {
  const destination = legacyServerAdminDestination(params.serverId, params.path, url.search);
  if (!destination) error(404, 'Not found');
  redirect(308, destination);
};
