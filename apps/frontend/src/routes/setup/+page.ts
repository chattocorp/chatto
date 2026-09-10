import { redirect } from '@sveltejs/kit';
import { getPublicServerInfo } from '$lib/api-client/server';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url }) => {
  if (!isBackendCapableOrigin(url)) redirect(302, '/login');
  // Always recheck: another visitor or replica can complete setup at any time.
  const setupServer = await getPublicServerInfo(url.origin);
  if (!setupServer.setupRequired) redirect(302, '/login');
  return { setupServer };
};
