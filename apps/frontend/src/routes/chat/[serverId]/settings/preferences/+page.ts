import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ url }) => {
  const returnTo = `${url.pathname.split('/settings/preferences')[0]}/settings`;
  redirect(307, `/settings?returnTo=${encodeURIComponent(returnTo)}`);
};
