import { redirect } from '@sveltejs/kit';
import { serverRegistry } from '$lib/client';
import { gutterEnabled } from '$lib/frontendConfig';
import type { PageLoad } from './$types';

export const load: PageLoad = () => {
  // A server that disables the gutter also hides the Server Directory.
  // Keep the page when the origin is unknown, so the page cannot redirect in a loop.
  if (!gutterEnabled && serverRegistry.originServer) redirect(302, '/');
};
