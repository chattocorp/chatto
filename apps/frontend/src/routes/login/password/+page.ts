import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
  const { serverInfo } = await parent();
  // An SSO-only server has no password login. Send the visitor to the provider choices.
  if (serverInfo?.directLoginEnabled === false) redirect(302, `/login${url.search}`);
};
