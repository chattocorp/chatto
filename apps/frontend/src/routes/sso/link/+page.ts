import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

/** Require the origin session before the link window talks to the account API. */
export const load: PageLoad = async ({ url, parent }) => {
  const { user, serverInfo } = await parent();
  if (!user) {
    redirect(302, `${resolve('/login')}?redirect=${encodeURIComponent(url.pathname + url.search)}`);
  }
  return { user, directLoginEnabled: serverInfo?.directLoginEnabled ?? false };
};
