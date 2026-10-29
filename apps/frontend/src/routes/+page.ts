import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { serverRegistry } from '$lib/client';

export const load: PageLoad = async ({ parent, url }) => {
  const { user } = await parent();
  if (user) redirect(302, `/chat${url.search}`);
  redirect(302, serverRegistry.originServer ? '/login' : '/chat/servers');
};
